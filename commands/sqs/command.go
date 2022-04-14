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
	metricSumZero             = 0.0
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
	log       *zap.SugaredLogger
	rootArgs  rootArguments
	awsClient *AWSClients
)

type Queue struct {
	URL  string
	Name string
}
type DeadLetterQueue struct {
	MainQueue       Queue
	DeadLetterQueue []Queue
}

type QueuesToClean struct {
	Queue           *Queue
	DeadLetterQueue *DeadLetterQueue
}

type AWSClients struct {
	CloudWatch *cloudwatch.CloudWatch
	SQS        *sqs.SQS
}

func init() {
	Command.Flags().StringVarP(&AwsSqsEndpoint, "sqs-endpoint", "", "", "SQS endpoint")
	Command.Flags().StringVarP(&AwsCloudWatchEndpoint, "cloudwatch-endpoint", "", "", "CloudWatch endpoint")
	Command.Flags().StringVarP(&AwsSQSQueuePrefix, "queue-prefix", "", "", "SQS queue prefix")
	Command.Flags().StringVarP(&CheckTagNameUpdateDate, "check-tag-name-update-date", "", "", "Define tag name for check update date")
	Command.Flags().StringVarP(&UnusedSince, "since", "", "7d", "Filter with keyword since=2h")
}

func printResult(_ *cobra.Command, _ []string) {
	awsClient = NewAWSClients()
	queuesAfterDelete := make(map[string][]string)
	queues := awsSQSToClean()
	table := tablewriter.NewWriter(os.Stdout)

	if rootArgs.enableDelete {
		if !rootArgs.noHeader {
			table.ClearRows()
			table.SetHeader([]string{"Queue", "Message"})
		}
		queuesAfterDelete = awsClient.AwsSQSDelete(queues)
	} else if !rootArgs.noHeader {
		table.SetHeader([]string{"Queue"})
		for k, v := range queues {
			queuesAfterDelete[k] = []string{v}
		}
	}

	for _, v := range queuesAfterDelete {
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

func NewAWSClients() *AWSClients {
	log.Debug("Initializing SQS session")
	sqsSvc := sqs.New(helpers.GetAwsSession(AwsSqsEndpoint))

	log.Debug("Initializing Cloudwatch session")
	cwSvc := cloudwatch.New(helpers.GetAwsSession(AwsCloudWatchEndpoint))
	return &AWSClients{
		CloudWatch: cwSvc,
		SQS:        sqsSvc,
	}
}

func awsSQSToClean() map[string]string {
	log.Debug("Listing queues")
	listQueues, err := awsClient.SQS.ListQueues(&sqs.ListQueuesInput{QueueNamePrefix: aws.String(AwsSQSQueuePrefix)})
	if err != nil {
		log.Fatal(err)
	}
	log.Debugw("Queues fetched", "queuesNb", len(listQueues.QueueUrls))

	waitGrp := &sync.WaitGroup{}
	waitGrp.Add(len(listQueues.QueueUrls))
	limiter := make(chan bool, maxProcessingWorkers)
	sqsToCleanChannel := make([]chan *QueuesToClean, len(listQueues.QueueUrls))

	for i, queueURL := range listQueues.QueueUrls {
		sqsToCleanChannel[i] = make(chan *QueuesToClean, 1)
		go processing(limiter, sqsToCleanChannel[i], waitGrp, awsClient, queueURL)
	}
	waitGrp.Wait()
	var queuesToCleanToMerge []QueuesToClean
	for i := 0; i < len(listQueues.QueueUrls); i++ {
		queuesToClean := <-sqsToCleanChannel[i]
		if queuesToClean != nil {
			queuesToCleanToMerge = append(queuesToCleanToMerge, *queuesToClean)
		}
	}

	sqsToClean := make(map[string]string)
	// add queues to clean
	for _, queuesToClean := range queuesToCleanToMerge {
		if queuesToClean.Queue != nil {
			sqsToClean[queuesToClean.Queue.URL] = queuesToClean.Queue.Name
		}
	}
	// add Dead Letter Queues to clean
	for _, queuesToClean := range queuesToCleanToMerge {
		if queuesToClean.DeadLetterQueue != nil {
			lenQueues := len(queuesToClean.DeadLetterQueue.DeadLetterQueue)
			lenQueuesMatched := 0
			for _, queue := range queuesToClean.DeadLetterQueue.DeadLetterQueue {
				if _, ok := sqsToClean[queue.URL]; ok {
					lenQueuesMatched++
				}
			}
			if lenQueuesMatched == lenQueues {
				sqsToClean[queuesToClean.Queue.URL] = queuesToClean.Queue.Name
			}
		}
	}
	return sqsToClean
}

func processing(limiter chan bool, sqsToClean chan *QueuesToClean, group *sync.WaitGroup, awsClients *AWSClients, queueURL *string) {
	now := time.Now()
	queueName := *sqsQueueURLToQueueName(queueURL)
	tLog := log.With("queueName", queueName)
	tLog.Debugw("start processing", "queueName", queueName)

	queuesToClean := &QueuesToClean{}
	limiter <- true
	// define the func to be executed at the end of the processing
	defer func() {
		<-limiter
		sqsToClean <- queuesToClean
		group.Done()
		tLog.Debugw("stop processing", "queueName", queueName)
	}()

	if queueURL != nil {
		if rootArgs.excludePatten != nil {
			if rootArgs.excludePatten.MatchString(queueName) {
				tLog.Debugw("Skipping queue due to exclude pattern ")
				return
			}
		}
		if CheckTagNameUpdateDate != "" {
			tLog.Debug("Updating SNS usage date tag")
			skip, err := updateDateToDelete(queueURL)
			if err != nil {
				log.Warnw("Cannot check tag name date update", "err", err)
			}
			if skip {
				log.Infof("SQS: Skipping (tag name %s): %s", CheckTagNameUpdateDate, *sqsQueueURLToQueueName(queueURL))
				return
			}
		}
		// Retrieve metrics from Cloud Watch
		metricsSum, err := awsClients.GetCloudWatchMetrics(&now, queueURL)
		if err != nil {
			tLog.Warnw("cannot get metrics", "err", err)
			return
		}

		// Evaluate if is dead letter queue and get associated queue
		isDeadLetterQueue, associatedQueue, err := awsClients.IsDeadLetterQueue(queueURL)
		if err != nil {
			tLog.Warnw("cannot get dead letter queue source Queues", "err", err)
		}

		tLog = tLog.With("metricsSum", metricsSum)
		if isDeadLetterQueue {
			var _associatedQueue []Queue
			for _, s := range associatedQueue {
				_associatedQueue = append(_associatedQueue, Queue{
					Name: *sqsQueueURLToQueueName(&s),
					URL:  s,
				})
			}
			queuesToClean.DeadLetterQueue = &DeadLetterQueue{
				MainQueue: Queue{
					URL:  *queueURL,
					Name: queueName,
				},
				DeadLetterQueue: _associatedQueue,
			}
		}
		if *metricsSum == metricSumZero {
			tLog.Debugw("Queue is unused", "queueName", queueName)
			queuesToClean.Queue = &Queue{
				URL:  *queueURL,
				Name: queueName,
			}
		} else {
			tLog.Debug("Queue is used")
		}
	}
}

func (c *AWSClients) IsDeadLetterQueue(queueURL *string) (bool, []string, error) {
	isDeadLetterQueue := false
	var associatedQueuesForDeadLetterQueue []string
	associatedQueues, err := c.SQS.ListDeadLetterSourceQueues(&sqs.ListDeadLetterSourceQueuesInput{QueueUrl: queueURL})
	if err != nil {
		return false, associatedQueuesForDeadLetterQueue, fmt.Errorf("cannot get dead letter queue source Queues: %w", err)
	}
	if len(associatedQueues.QueueUrls) != 0 {
		isDeadLetterQueue = true
	}
	for _, associatedQueue := range associatedQueues.QueueUrls {
		if associatedQueue != nil {
			associatedQueuesForDeadLetterQueue = append(associatedQueuesForDeadLetterQueue, *associatedQueue)
		}
	}
	return isDeadLetterQueue, associatedQueuesForDeadLetterQueue, nil
}

func (c *AWSClients) GetCloudWatchMetrics(now *time.Time, queueURL *string) (*float64, error) {
	tLog := log.With("queueURL", queueURL)
	metrics, err := c.CloudWatch.GetMetricData(GetSQSMetricDataInput(rootArgs.UnusedSinceDate, now, queueURL))
	if err != nil {
		return nil, fmt.Errorf("cannot get metrics: %w", err)
	}
	metricsSum := 0.0
	for _, result := range metrics.MetricDataResults {
		if len(result.Values) != 0 {
			for k, value := range result.Values {
				log.Debugw(
					"Metric fetched",
					"metricLabel", *result.Label,
					"metricValue", *value,
					"metricDate", *result.Timestamps[k],
				)
				metricsSum += *value
			}
		} else {
			tLog.Debugw("No metric value", "metricLabel", *result.Label)
		}
	}
	return &metricsSum, nil
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
	queueTags, err := awsClient.SQS.ListQueueTags(&sqs.ListQueueTagsInput{QueueUrl: queueURL})
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

func (c *AWSClients) AwsSQSDelete(queues map[string]string) map[string][]string {
	sqsToDelete := make(map[string][]string)
	for queue := range queues {
		log.Debugw("Deleting queue ", "queueName", queue)
		_, err := awsClient.SQS.DeleteQueue(&sqs.DeleteQueueInput{QueueUrl: &queue})
		sqsToDelete[queue] = []string{*sqsQueueURLToQueueName(&queue), func(err error) string {
			if err != nil {
				return err.Error()
			}
			return "OK"
		}(err)}
	}
	return sqsToDelete
}
