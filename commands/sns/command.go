// Package sqs implements the 'aws sns'  sub-command.
package sns

import (
	"log"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/aws/aws-sdk-go/service/sns"
	"github.com/habx/aws-mq-cleaner/flags"
	"github.com/habx/aws-mq-cleaner/helpers"
	"github.com/habx/aws-mq-cleaner/logger"
	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

const (
	subscriptionsPending   = "SubscriptionsPending"
	subscriptionsConfirmed = "SubscriptionsConfirmed"
	subscriptionsSumZero   = 0
)

var (
	// Command performs the "list clusters" function
	Command = &cobra.Command{
		Use:     "sns",
		Aliases: []string{"sns"},
		Short:   "AWS SNS",
		Long:    `This command will identify the topic to be cleaned. `,
		PreRun:  validation,
		Run:     runCommand,
	}
	l        *zap.Logger
	rootArgs rootArguments
	snsSvc   *sns.SNS
	mux      sync.RWMutex
)

type rootArguments struct {
	enableDelete  bool
	noHeader      bool
	excludePatten *regexp.Regexp
}

func defaultArguments() rootArguments {
	excludePatten, err := helpers.InitExcludePattern(flags.ExcludePatten)
	if err != nil {
		l.Sugar().Fatalf(err.Error())
	}
	return rootArguments{
		enableDelete:  flags.Delete,
		noHeader:      flags.NoHeader,
		excludePatten: excludePatten,
	}
}

func init() {
	Command.Flags().StringVarP(&AwsSnsEndpoint, "sns-endpoint", "", "", "SNS endpoint")
	Command.Flags().StringVarP(&AwsSnsTopicPrefix, "topic-prefix", "", "", "SQS queue prefix")
	Command.Flags().IntVarP(&AwsSnsMaxTopic, "max-topics", "", 100, "Get max topics (Example: 10) before filtering")
}

func validation(cmd *cobra.Command, cmdLineArgs []string) {
	l = logger.GetLogger(flags.LogLevel)
	rootArgs = defaultArguments()
	l.Debug("--sns-endpoint", zap.String("AwsSnsEndpoint", AwsSnsEndpoint))
	l.Debug("--max-topics", zap.Int("AwsSnsMaxTopic", AwsSnsMaxTopic))
	l.Debug("--topic-prefix", zap.String("AwsSnsTopicPrefix", AwsSnsTopicPrefix))
	l.Debug("--delete", zap.Bool("delete", rootArgs.enableDelete))
	l.Debug("--no-header", zap.Bool("no-header", rootArgs.noHeader))
	if rootArgs.excludePatten != nil {
		l.Debug("--exclude-patten", zap.String("AwsSnsTopicPrefix", rootArgs.excludePatten.String()))
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
	} else {
		if !rootArgs.noHeader {
			table.SetHeader([]string{"Topic"})
		}
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
	l.Sugar().Debug("AWS: init sns session")
	snsSvc = sns.New(helpers.GetAwsSession(AwsSnsEndpoint))

	var nextToken *string
	var topics []*sns.Topic
	m := make(map[string][]string)
	for {
		topicsReq, err := snsSvc.ListTopics(&sns.ListTopicsInput{NextToken: nextToken})
		if err != nil {
			l.Sugar().Fatal(err)
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
					l.Sugar().Debug("SNS: match prefix (", *topic.TopicArn, ")")
					_topics = append(_topics, topic)
				}
			}
		}
		topics = _topics
	}

	l.Sugar().Debug("AWS: before truncate list topics len (", len(topics), ")")
	if AwsSnsMaxTopic < len(topics) {
		topics = topics[:AwsSnsMaxTopic]
	}
	l.Sugar().Debug("AWS: list topics len (", len(topics), ")")
	var wg sync.WaitGroup
	wg.Add(len(topics))
	for _, topic := range topics {
		go func(topicARN *string) {
			if rootArgs.excludePatten != nil {
				if rootArgs.excludePatten.MatchString(*snsTopicARNToTopicName(topicARN)) {
					l.Sugar().Debug("SNS: Skipping (exclude patten) " + *snsTopicARNToTopicName(topicARN))
					wg.Done()
					return
				}
			}
			if topicARN != nil {
				l.Sugar().Debug("SNS: ", *topicARN)
				topicAttributes, err := snsSvc.GetTopicAttributes(&sns.GetTopicAttributesInput{TopicArn: topicARN})
				if err != nil {
					l.Sugar().Fatal(err)
				}
				topicSubscriptionsSum := 0
				if topicAttributes != nil {
					for topicAttributesName, topicAttributesValue := range topicAttributes.Attributes {
						// subscriptionsConfirmed
						l.Sugar().Debug("SNS: "+topicAttributesName, "/", *topicAttributesValue)
						if topicAttributesName == subscriptionsConfirmed || topicAttributesName == subscriptionsPending {
							l.Sugar().Debug("SNS: ", *topicARN, "/", topicAttributesName, " : ", *topicAttributesValue)
							if topicAttributesValue != nil {
								value, err := strconv.Atoi(*topicAttributesValue)
								if err != nil {
									log.Fatal("cannot parse ", topicAttributesName, " value")
								}
								topicSubscriptionsSum += value
							}
						}
					}
					if topicSubscriptionsSum == subscriptionsSumZero {
						l.Sugar().Debug("SNS: ", *topicARN, " unused")
						mux.Lock()
						m[*topicARN] = []string{*snsTopicARNToTopicName(topicARN)}
						mux.Unlock()
					} else {
						l.Sugar().Debug("SNS: ", *topicARN, " used")
					}
				}
			}
			wg.Done()
		}(topic.TopicArn)
	}
	wg.Wait()
	return m
}
func snsTopicARNToTopicName(topicARN *string) *string {
	split := strings.Split(*topicARN, ":")
	return &split[len(split)-1]
}

func awsSNSDelete(topics map[string][]string) map[string][]string {
	m := make(map[string][]string)
	for topicARN := range topics {
		l.Sugar().Debug("SNS: delete ", topicARN)
		_, err := snsSvc.DeleteTopic(&sns.DeleteTopicInput{TopicArn: &topicARN})
		m[topicARN] = []string{*snsTopicARNToTopicName(&topicARN), func(err error) string {
			if err != nil {
				return err.Error()
			}
			return "OK"
		}(err)}
	}
	return m
}
