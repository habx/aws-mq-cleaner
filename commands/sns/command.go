// Package sqs implements the 'aws sns' sub-command.
package sns

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go/service/sns"
	"github.com/habx/aws-mq-cleaner/flags"
	"github.com/habx/aws-mq-cleaner/helpers"
	"github.com/habx/aws-mq-cleaner/logger"
	t "github.com/habx/aws-mq-cleaner/time"
	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

const (
	subscriptionsPending   = "SubscriptionsPending"
	subscriptionsConfirmed = "SubscriptionsConfirmed"
	subscriptionsSumZero   = 0
	maxEpic                = 100
)

var (
	// Command performs the "list clusters" function.
	Command = &cobra.Command{
		Use:     "sns",
		Aliases: []string{"sns"},
		Short:   "AWS SNS",
		Long:    `This command will identify the topic to be cleaned. `,
		PreRun:  validation,
		Run:     runCommand,
	}
	log      *zap.SugaredLogger
	rootArgs rootArguments
	snsSvc   *sns.SNS
	mux      sync.RWMutex
)

type rootArguments struct {
	UnusedSince     string
	UnusedSinceDate *time.Time
	enableDelete    bool
	noHeader        bool
	excludePatten   *regexp.Regexp
}

func defaultArguments() rootArguments {
	unusedSinceDate, err := t.ParseSince(UnusedSince)
	if err != nil {
		log.Error("missing params --since")
		log.Fatalf(err.Error())
	}
	excludePatten, err := helpers.InitExcludePattern(flags.ExcludePatten)
	if err != nil {
		log.Fatalf(err.Error())
	}
	return rootArguments{
		UnusedSince:     UnusedSince,
		UnusedSinceDate: unusedSinceDate,
		enableDelete:    flags.Delete,
		noHeader:        flags.NoHeader,
		excludePatten:   excludePatten,
	}
}

func init() {
	Command.Flags().StringVarP(&AwsSnsEndpoint, "sns-endpoint", "", "", "SNS endpoint")
	Command.Flags().StringVarP(&CheckTagNameUpdateDate, "check-tag-name-update-date", "", "", "Define tag name for check update date")
	Command.Flags().StringVarP(&UnusedSince, "since", "", "7d", "Used for 'update date' AWS tag")
	Command.Flags().StringVarP(&AwsSnsTopicPrefix, "topic-prefix", "", "", "SQS queue prefix")
	Command.Flags().IntVarP(&AwsSnsMaxTopic, "max-topics", "", maxEpic, "Get max topics (Example: 10) before filtering")
}

func validation(cmd *cobra.Command, cmdLineArgs []string) {
	log = logger.GetLogger(flags.LogLevel).Sugar()
	rootArgs = defaultArguments()
	log.Debugw("--check-tag-name-update-date", "CheckTagNameUpdateDate", CheckTagNameUpdateDate)
	log.Debugw("--sns-endpoint", "AwsSnsEndpoint", AwsSnsEndpoint)
	log.Debugw("--since", "sinceParsed", *rootArgs.UnusedSinceDate, "since", rootArgs.UnusedSince)
	log.Debugw("--max-topics", "AwsSnsMaxTopic", AwsSnsMaxTopic)
	log.Debugw("--topic-prefix", "AwsSnsTopicPrefix", AwsSnsTopicPrefix)
	log.Debugw("--delete", "delete", rootArgs.enableDelete)
	log.Debugw("--no-header", "no-header", rootArgs.noHeader)
	if rootArgs.excludePatten != nil {
		log.Debugw("--exclude-patten", "AwsSnsTopicPrefix", rootArgs.excludePatten.String())
	}
}

func runCommand(cmd *cobra.Command, cmdLineArgs []string) {
	topics := awsSNSToClean()

	table := tablewriter.NewWriter(os.Stdout)

	if rootArgs.enableDelete {
		if !rootArgs.noHeader {
			table.ClearRows()
			table.SetHeader([]string{"Topic", "Message"})
		}
		topics = awsSNSDelete(topics)
	} else if !rootArgs.noHeader {
		table.SetHeader([]string{"Topic"})
	}

	for _, topic := range topics {
		table.Append(topic)
	}
	table.SetAutoWrapText(false)
	table.SetAutoFormatHeaders(true)
	table.SetHeaderAlignment(tablewriter.ALIGN_LEFT)
	table.SetAlignment(tablewriter.ALIGN_LEFT)
	table.SetCenterSeparator("")
	table.SetColumnSeparator("")
	table.SetRowSeparator("")
	table.SetHeaderLine(false)
	table.SetBorder(false)
	table.SetTablePadding("\t") // pad with tabs
	table.SetNoWhiteSpace(true)
	table.Render()
}

func awsSNSToClean() map[string][]string {
	log.Debug("AWS: init sns session")
	snsSvc = sns.New(helpers.GetAwsSession(AwsSnsEndpoint))

	var nextToken *string
	var topics []*sns.Topic
	snsToClean := make(map[string][]string)
	for {
		topicsReq, err := snsSvc.ListTopics(&sns.ListTopicsInput{NextToken: nextToken})
		if err != nil {
			log.Fatal(err)
		}
		topics = append(topics, topicsReq.Topics...)
		if topicsReq.NextToken != nil {
			if AwsSnsMaxTopic <= len(topics) {
				break
			}
			nextToken = topicsReq.NextToken
		} else {
			break
		}
	}

	if AwsSnsTopicPrefix != "" {
		var _topics []*sns.Topic
		for _, topic := range topics {
			if topic.TopicArn != nil {
				if regexp.MustCompile(`^` + AwsSnsTopicPrefix).MatchString(*snsTopicARNToTopicName(topic.TopicArn)) {
					log.Debug("SNS: match prefix (", *topic.TopicArn, ")")
					_topics = append(_topics, topic)
				}
			}
		}
		topics = _topics
	}

	log.Debug("AWS: before truncate list topics len (", len(topics), ")")
	if AwsSnsMaxTopic < len(topics) {
		topics = topics[:AwsSnsMaxTopic]
	}
	log.Debug("AWS: list topics len (", len(topics), ")")
	var waitGrp sync.WaitGroup
	waitGrp.Add(len(topics))
	for _, topic := range topics {
		go func(topicARN *string) {
			if rootArgs.excludePatten != nil {
				if rootArgs.excludePatten.MatchString(*snsTopicARNToTopicName(topicARN)) {
					log.Debug("SNS: Skipping (exclude patten) " + *snsTopicARNToTopicName(topicARN))
					waitGrp.Done()
					return
				}
			}
			if topicARN != nil {
				log.Debug("SNS: ", *topicARN)
				topicAttributes, err := snsSvc.GetTopicAttributes(&sns.GetTopicAttributesInput{TopicArn: topicARN})
				if err != nil {
					log.Warnw("cannot get topic attributes", "err", err)
					waitGrp.Done()
					return
				}
				if CheckTagNameUpdateDate != "" {
					log.Debug("AWS: update date tag enabled")
					skip, err := updateDateToDelete(topicARN)
					if err != nil {
						log.Warnf("cannot check tag name date update: %v", err)
					}
					if skip {
						log.Infof("SQS: Skipping (tag name %s): %s", CheckTagNameUpdateDate, *snsTopicARNToTopicName(topicARN))
						waitGrp.Done()
						return
					}
				}
				topicSubscriptionsSum := 0
				if topicAttributes != nil {
					waitGrp.Done()
					return
				}
				for topicAttributesName, topicAttributesValue := range topicAttributes.Attributes {
					// subscriptionsConfirmed
					log.Debug("SNS: "+topicAttributesName, "/", *topicAttributesValue)
					if topicAttributesName == subscriptionsConfirmed || topicAttributesName == subscriptionsPending {
						if topicAttributesValue != nil {
							log.Debug("SNS: ", *topicARN, "/", topicAttributesName, " : ", *topicAttributesValue)
							value, err := strconv.Atoi(*topicAttributesValue)
							if err != nil {
								log.Errorf("cannot parse %s value", topicAttributesName)
								continue
							}
							topicSubscriptionsSum += value
						}
					}
				}
				if topicSubscriptionsSum == subscriptionsSumZero {
					log.Debug("SNS: ", *topicARN, " unused")
					mux.Lock()
					snsToClean[*topicARN] = []string{*snsTopicARNToTopicName(topicARN)}
					mux.Unlock()
				} else {
					log.Debug("SNS: ", *topicARN, " used")
				}
			}
			waitGrp.Done()
		}(topic.TopicArn)
	}
	waitGrp.Wait()
	return snsToClean
}

func snsTopicARNToTopicName(topicARN *string) *string {
	split := strings.Split(*topicARN, ":")
	return &split[len(split)-1]
}

// return true if topic should be skipped.
func updateDateToDelete(topicArn *string) (bool, error) {
	snsTags, err := snsSvc.ListTagsForResource(&sns.ListTagsForResourceInput{ResourceArn: topicArn})
	if err != nil {
		return false, fmt.Errorf("cannot list tags for ressource '%s' : %w", *topicArn, err)
	}
	for _, tag := range snsTags.Tags {
		if tag.Key != nil && *tag.Key == CheckTagNameUpdateDate {
			if tag.Value == nil {
				return false, nil
			}
			dataTime, err := time.Parse("2006-01-02T15:04:05.000-03:00", *tag.Value)
			if err != nil {
				return false, fmt.Errorf("cannot parse datetime tags '%s' : %w", *tag.Value, err)
			}
			if dataTime.After(*rootArgs.UnusedSinceDate) {
				log.Infow("topic is not old enough to delete", "topicArn", *topicArn, "date", dataTime.String())
				return true, nil
			}
			log.Infow("topic is old enough to delete", "topicArn", *topicArn, "date", dataTime.String())
			return false, nil
		}
	}
	return false, nil
}

func awsSNSDelete(topics map[string][]string) map[string][]string {
	snsToDelete := make(map[string][]string)
	for topicARN := range topics {
		log.Debug("SNS: delete ", topicARN)
		_, err := snsSvc.DeleteTopic(&sns.DeleteTopicInput{TopicArn: &topicARN})
		snsToDelete[topicARN] = []string{*snsTopicARNToTopicName(&topicARN), func(err error) string {
			if err != nil {
				return err.Error()
			}
			return "OK"
		}(err)}
	}
	return snsToDelete
}
