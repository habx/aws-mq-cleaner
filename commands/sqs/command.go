// Package sqs implements the 'aws sqs'  sub-command.
package sqs

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/cloudwatch"
	"github.com/aws/aws-sdk-go/service/sqs"
	"github.com/habx/aws-mq-cleaner/flags"
	"github.com/habx/aws-mq-cleaner/helpers"
	"github.com/habx/aws-mq-cleaner/logger"
	t "github.com/habx/aws-mq-cleaner/time"
	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

const (
	metricSumZero             = 0
	maxCloudWatchMaxDatapoint = 100
)

var (
	// Command performs the "list clusters" function.
	Command = &cobra.Command{
		Use:     "sqs",
		Aliases: []string{"sqs"},
		Short:   "AWS SQS",
		Long:    `This command will identify the queues to be cleaned.`,
		PreRun:  validation,
		Run:     printResult,
	}
	log      *zap.SugaredLogger
	rootArgs rootArguments
	sqsSvc   *sqs.SQS
	mux      sync.RWMutex
)

func init() {
	Command.Flags().StringVarP(&AwsSqsEndpoint, "sqs-endpoint", "", "", "SQS endpoint")
	Command.Flags().StringVarP(&AwsCloudWatchEndpoint, "cloudwatch-endpoint", "", "", "CloudWatch endpoint")
	Command.Flags().StringVarP(&AwsSQSQueuePrefix, "queue-prefix", "", "", "SQS queue prefix")
	Command.Flags().StringVarP(&CheckTagNameUpdateDate, "check-tag-name-update-date", "", "", "Define tag name for check update date")
	Command.Flags().StringVarP(&UnusedSince, "since", "", "7d", "Filter with keyword since=2h")
}

func printResult(cmd *cobra.Command, cmdLineArgs []string) {
	queues := awsSQSToClean()

	table := tablewriter.NewWriter(os.Stdout)

	if rootArgs.enableDelete {
		if !rootArgs.noHeader {
			table.ClearRows()
			table.SetHeader([]string{"Queue", "Message"})
		}
		queues = awsSQSDelete(queues)
	} else if !rootArgs.noHeader {
		table.SetHeader([]string{"Queue"})
	}

	for _, v := range queues {
		table.Append(v)
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

func awsSQSToClean() map[string][]string {
	log.Debug("AWS: init sqs session")
	sqsSvc = sqs.New(helpers.GetAwsSession(AwsSqsEndpoint))

	log.Debug("AWS: init cloudwatch session")
	cwSvc := cloudwatch.New(helpers.GetAwsSession(AwsCloudWatchEndpoint))
	now := time.Now()
	log.Debug("AWS: list queues")
	listQueues, err := sqsSvc.ListQueues(&sqs.ListQueuesInput{QueueNamePrefix: aws.String(AwsSQSQueuePrefix)})
	if err != nil {
		log.Fatal(err)
	}
	log.Debugf("AWS: queues number %d", len(listQueues.QueueUrls))
	sqsToClean := make(map[string][]string)

	var waitGrp sync.WaitGroup
	waitGrp.Add(len(listQueues.QueueUrls))

	for _, queueURL := range listQueues.QueueUrls {
		go func(qURL *string) {
			if qURL != nil {
				if rootArgs.excludePatten != nil {
					if rootArgs.excludePatten.MatchString(*sqsQueueURLToQueueName(qURL)) {
						log.Debug("SQS: Skipping (exclude patten) " + *sqsQueueURLToQueueName(qURL))
						waitGrp.Done()
						return
					}
				}
				if CheckTagNameUpdateDate != "" {
					log.Debug("AWS: update date tag enabled")
					skip, err := updateDateToDelete(qURL)
					if err != nil {
						log.Warnf("cannot check tag name date update: %v", err)
					}
					if skip {
						log.Infof("SQS: Skipping (tag name %s): %s", CheckTagNameUpdateDate, *sqsQueueURLToQueueName(qURL))
						waitGrp.Done()
						return
					}
				}
				metrics, err := cwSvc.GetMetricData(GetSQSMetricDataInput(rootArgs.UnusedSinceDate, &now, qURL))
				if err != nil {
					log.Fatal(err)
				}
				metricsSum := 0.0
				failed := false
				for _, result := range metrics.MetricDataResults {
					if len(result.Values) != 0 {
						for _, value := range result.Values {
							log.Debug("SQS: ("+*sqsQueueURLToQueueName(qURL)+"/"+*result.Label+") Value : ", *value)
							metricsSum += *value
						}
					} else {
						log.Debug("SQS: ("+*sqsQueueURLToQueueName(qURL)+"/"+*result.Label+") No value : ", *result.Label)
						failed = true
					}
				}
				if !failed {
					if metricsSum == metricSumZero {
						log.Debug("SQS: "+*sqsQueueURLToQueueName(qURL)+" is unused (metricsSum=", metricsSum, ")")
						mux.Lock()
						sqsToClean[*qURL] = []string{*sqsQueueURLToQueueName(qURL)}
						mux.Unlock()
					} else {
						log.Debug("SQS: "+*sqsQueueURLToQueueName(qURL)+" is used (metricsSum=", metricsSum, ")")
					}
				} else {
					log.Debug("SQS: " + *sqsQueueURLToQueueName(qURL) + " failed, invalid metric")
				}
			}
			waitGrp.Done()
		}(queueURL)
	}
	waitGrp.Wait()
	return sqsToClean
}

type rootArguments struct {
	UnusedSince     string
	UnusedSinceDate *time.Time
	enableDelete    bool
	noHeader        bool
	excludePatten   *regexp.Regexp
}

func defaultAwsSQSArguments() rootArguments {
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

func validation(cmd *cobra.Command, cmdLineArgs []string) {
	log = logger.GetLogger(flags.LogLevel).Sugar()
	rootArgs = defaultAwsSQSArguments()
	log.Debugw("--since", "sinceParsed", *rootArgs.UnusedSinceDate, "since", rootArgs.UnusedSince)
	log.Debugw("--queue-prefix", "AwsSQSQueuePrefix", AwsSQSQueuePrefix)
	log.Debugw("--sqs-endpoint", "AwsSqsEndpoint", AwsSqsEndpoint)
	log.Debugw("--cloudwatch-endpoint", "AwsCloudWatchEndpoint", AwsCloudWatchEndpoint)
	log.Debugw("--delete", "delete", rootArgs.enableDelete)
	log.Debugw("--no-header", "no-header", rootArgs.noHeader)
	if rootArgs.excludePatten != nil {
		log.Debug("--exclude-patten", "AwsSnsTopicPrefix", rootArgs.excludePatten.String())
	}
	if rootArgs.UnusedSince == "" {
		log.Fatal("missing --since")
	}
}

func GetSQSMetricDataInput(start *time.Time, end *time.Time, queueURL *string) *cloudwatch.GetMetricDataInput {
	if log != nil {
		log.Debugf("Time: delta period %d min", int64(end.Sub(*start).Minutes()))
		log.Debugf("Time: start %s", start.UTC().String())
		log.Debugf("Time: end %s", end.UTC().String())
		log.Debugf("SQS: queueName %s", *sqsQueueURLToQueueName(queueURL))
	}
	return &cloudwatch.GetMetricDataInput{
		EndTime:       end,
		MaxDatapoints: aws.Int64(maxCloudWatchMaxDatapoint),
		MetricDataQueries: []*cloudwatch.MetricDataQuery{
			{
				Id: aws.String("i0"),
				MetricStat: &cloudwatch.MetricStat{
					Metric: &cloudwatch.Metric{
						Dimensions: []*cloudwatch.Dimension{{
							Name:  aws.String("QueueName"),
							Value: sqsQueueURLToQueueName(queueURL),
						}},
						MetricName: aws.String("NumberOfMessagesReceived"),
						Namespace:  aws.String("AWS/SQS"),
					},
					Period: aws.Int64(int64(end.Sub(*start).Minutes())),
					Stat:   aws.String("Sum"),
					Unit:   aws.String("Count"),
				},
				ReturnData: aws.Bool(true),
			},
			{
				Id: aws.String("i1"),
				MetricStat: &cloudwatch.MetricStat{
					Metric: &cloudwatch.Metric{
						Dimensions: []*cloudwatch.Dimension{{
							Name:  aws.String("QueueName"),
							Value: sqsQueueURLToQueueName(queueURL),
						}},
						MetricName: aws.String("NumberOfEmptyReceives"),
						Namespace:  aws.String("AWS/SQS"),
					},
					Period: aws.Int64(int64(end.Sub(*start).Minutes())),
					Stat:   aws.String("Sum"),
					Unit:   aws.String("Count"),
				},
				ReturnData: aws.Bool(true),
			},
		},
		StartTime: start,
	}
}

func sqsQueueURLToQueueName(queueURL *string) *string {
	split := strings.Split(*queueURL, "/")
	return &split[len(split)-1]
}

// return true if the topic should be skipped.
func updateDateToDelete(queueURL *string) (bool, error) {
	queueTags, err := sqsSvc.ListQueueTags(&sqs.ListQueueTagsInput{QueueUrl: queueURL})
	if err != nil {
		return false, fmt.Errorf("cannot list queue tags '%s': %w", *queueURL, err)
	}
	for name, value := range queueTags.Tags {
		if name == CheckTagNameUpdateDate {
			if value == nil {
				return false, nil
			}
			dataTime, err := time.Parse("2006-01-02T15:04:05.000-03:00", *value)
			if err != nil {
				return false, fmt.Errorf("cannot parse datetime tags '%s' : %w", *value, err)
			}
			if dataTime.After(*rootArgs.UnusedSinceDate) {
				log.Infow("queue is not old enough to delete", "queueURL", *queueURL, "date", dataTime.String())
				return true, nil
			}
			log.Infow("queue is old enough to delete", "queueURL", *queueURL, "date", dataTime.String())
			return false, nil
		}
	}
	return false, nil
}

func awsSQSDelete(queues map[string][]string) map[string][]string {
	sqsToDelete := make(map[string][]string)
	for queue := range queues {
		log.Debug("SQS: delete ", queue)
		_, err := sqsSvc.DeleteQueue(&sqs.DeleteQueueInput{QueueUrl: &queue})
		sqsToDelete[queue] = []string{*sqsQueueURLToQueueName(&queue), func(err error) string {
			if err != nil {
				return err.Error()
			}
			return "OK"
		}(err)}
	}
	return sqsToDelete
}
