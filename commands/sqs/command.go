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
	maxCloudWatchMaxDatapoint = 100
	maxProcessingWorkers      = 5
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
)

func init() {
	Command.Flags().StringVarP(&AwsSqsEndpoint, "sqs-endpoint", "", "", "SQS endpoint")
	Command.Flags().StringVarP(&AwsCloudWatchEndpoint, "cloudwatch-endpoint", "", "", "CloudWatch endpoint")
	Command.Flags().StringVarP(&AwsSQSQueuePrefix, "queue-prefix", "", "", "SQS queue prefix")
	Command.Flags().StringVarP(&CheckTagNameUpdateDate, "check-tag-name-update-date", "", "", "Define tag name for check update date")
	Command.Flags().StringVarP(&UnusedSince, "since", "", "7d", "Filter with keyword since=2h")
}

func printResult(_ *cobra.Command, _ []string) {
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
	log.Debug("Initializing SQS session")
	sqsSvc = sqs.New(helpers.GetAwsSession(AwsSqsEndpoint))

	log.Debug("Initializing Cloudwatch session")
	cwSvc := cloudwatch.New(helpers.GetAwsSession(AwsCloudWatchEndpoint))

	log.Debug("Listing queues")
	listQueues, err := sqsSvc.ListQueues(&sqs.ListQueuesInput{QueueNamePrefix: aws.String(AwsSQSQueuePrefix)})
	if err != nil {
		log.Fatal(err)
	}
	log.Debugw("Queues fetched", "queuesNb", len(listQueues.QueueUrls))
	sqsToClean := make(map[string][]string)

	waitGrp := &sync.WaitGroup{}
	waitGrp.Add(len(listQueues.QueueUrls))
	limiter := make(chan bool, maxProcessingWorkers)
	sqsToCleanChannel := make([]chan map[string][]string, len(listQueues.QueueUrls))

	for i, queueURL := range listQueues.QueueUrls {
		sqsToCleanChannel[i] = make(chan map[string][]string, 1)
		go processing(limiter, sqsToCleanChannel[i], waitGrp, cwSvc, queueURL)
	}
	waitGrp.Wait()
	for i := 0; i < len(listQueues.QueueUrls); i++ {
		for k, v := range <-sqsToCleanChannel[i] {
			sqsToClean[k] = v
		}
	}

	return sqsToClean
}

func processing(limiter chan bool, sqsToClean chan map[string][]string, group *sync.WaitGroup, cwSvc *cloudwatch.CloudWatch, queueURL *string) {
	now := time.Now()
	queueName := *sqsQueueURLToQueueName(queueURL)
	qLog := log.With("queueName", queueName)
	qLog.Debugw("start processing", "queueName", queueName)
	_sqsToClean := make(map[string][]string)
	limiter <- true
	defer func() {
		<-limiter
		sqsToClean <- _sqsToClean
		group.Done()
		qLog.Debugw("stop processing", "queueName", queueName)
	}()

	if queueURL != nil {
		if rootArgs.excludePatten != nil {
			if rootArgs.excludePatten.MatchString(queueName) {
				qLog.Debugw("Skipping queue due to exclude pattern ")
				return
			}
		}
		if CheckTagNameUpdateDate != "" {
			qLog.Debug("Updating SNS usage date tag")
			skip, err := updateDateToDelete(queueURL)
			if err != nil {
				log.Warnw("Cannot check tag name date update", "err", err)
			}
			if skip {
				log.Infof("SQS: Skipping (tag name %s): %s", CheckTagNameUpdateDate, *sqsQueueURLToQueueName(queueURL))
				return
			}
		}
		metrics, err := cwSvc.GetMetricData(GetSQSMetricDataInput(rootArgs.UnusedSinceDate, &now, queueURL))
		if err != nil {
			log.Fatal(err)
		}
		metricsSum := 0.0
		for _, result := range metrics.MetricDataResults {
			if len(result.Values) != 0 {
				for k, value := range result.Values {
					qLog.Debugw(
						"Metric fetched",
						"metricLabel", *result.Label,
						"metricValue", *value,
						"metricDate", *result.Timestamps[k],
					)
					metricsSum += *value
				}
			}
		}

		// If we don't have any metric we shall just see if it's a queue
		if metricsSum == 0 {
			resp, err := sqsSvc.ListDeadLetterSourceQueues(&sqs.ListDeadLetterSourceQueuesInput{QueueUrl: queueURL})
			if err != nil {
				qLog.Warnw("Cannot list dead letter source queues", "err", err)
				return
			}
			metricsSum += float64(len(resp.QueueUrls))
			if metricsSum > 0 {
				qLog.Debugw("Queue is a dead letter queue", "sourceQueue", resp.QueueUrls[0])
			}
		}

		qLog = qLog.With("metricsSum", metricsSum)
		if metricsSum == 0 {
			qLog.Debug("Queue is unused")
			_sqsToClean[*queueURL] = []string{queueName}
		} else {
			qLog.Debug("Queue is used")
		}
	}
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

func validation(_ *cobra.Command, _ []string) {
	log = logger.GetLogger(flags.LogLevel).Sugar().With("command", "sqs")
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
			dataTime, err := time.Parse(time.RFC3339, *value)
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
		log.Debugw("Deleting queue ", "queuName", queue)
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
