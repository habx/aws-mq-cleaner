// Package sqs implements the 'aws sqs'  sub-command.
package sqs

import (
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/habx/aws-mq-cleaner/helpers"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/cloudwatch"
	"github.com/aws/aws-sdk-go/service/sqs"
	"github.com/habx/aws-mq-cleaner/flags"
	"github.com/habx/aws-mq-cleaner/logger"
	t "github.com/habx/aws-mq-cleaner/time"
	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

const metricSumZero = 0
const maxCloudWatchMaxDatapoint = 100

var (
	// Command performs the "list clusters" function
	Command = &cobra.Command{
		Use:     "sqs",
		Aliases: []string{"sqs"},
		Short:   "AWS SQS",
		Long:    `This command will identify the queues to be cleaned.`,
		PreRun:  validation,
		Run:     printResult,
	}
	l        *zap.Logger
	rootArgs rootArguments
	sqsSvc   *sqs.SQS
	mux      sync.RWMutex
)

func init() {
	Command.Flags().StringVarP(&AwsSqsEndpoint, "sqs-endpoint", "", "", "SQS endpoint")
	Command.Flags().StringVarP(&AwsCloudWatchEndpoint, "cloudwatch-endpoint", "", "", "CloudWatch endpoint")
	Command.Flags().StringVarP(&AwsSQSQueuePrefix, "queue-prefix", "", "", "SQS queue prefix")
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
	} else {
		if !rootArgs.noHeader {
			table.SetHeader([]string{"Queue"})
		}
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
	l.Sugar().Debug("AWS: init sqs session")
	sqsSvc = sqs.New(helpers.GetAwsSession(AwsSqsEndpoint))

	l.Sugar().Debug("AWS: init cloudwatch session")
	cwSvc := cloudwatch.New(helpers.GetAwsSession(AwsCloudWatchEndpoint))
	now := time.Now()
	listQueues, err := sqsSvc.ListQueues(&sqs.ListQueuesInput{QueueNamePrefix: aws.String(AwsSQSQueuePrefix)})
	if err != nil {
		l.Sugar().Fatal(err)
	}
	m := make(map[string][]string)

	var wg sync.WaitGroup
	wg.Add(len(listQueues.QueueUrls))

	for _, queueURL := range listQueues.QueueUrls {
		go func(qURL *string) {
			if qURL != nil {
				if rootArgs.excludePatten != nil {
					if rootArgs.excludePatten.MatchString(*sqsQueueURLToQueueName(qURL)) {
						l.Sugar().Debug("SQS: Skipping (exclude patten) " + *sqsQueueURLToQueueName(qURL))
						wg.Done()
						return
					}
				}
				metrics, err := cwSvc.GetMetricData(GetSQSMetricDataInput(rootArgs.UnusedSinceDate, &now, qURL))
				if err != nil {
					l.Sugar().Fatal(err)
				}
				metricsSum := 0.0
				failed := false
				for _, result := range metrics.MetricDataResults {
					if len(result.Values) != 0 {
						for _, value := range result.Values {
							l.Sugar().Debug("SQS: ("+*sqsQueueURLToQueueName(qURL)+"/"+*result.Label+") Value : ", *value)
							metricsSum += *value
						}
					} else {
						l.Sugar().Debug("SQS: ("+*sqsQueueURLToQueueName(qURL)+"/"+*result.Label+") No value : ", *result.Label)
						failed = true
					}
				}
				if !failed {
					if metricsSum == metricSumZero {
						l.Sugar().Debug("SQS: "+*sqsQueueURLToQueueName(qURL)+" is unused (metricsSum=", metricsSum, ")")
						mux.Lock()
						m[*qURL] = []string{*sqsQueueURLToQueueName(qURL)}
						mux.Unlock()
					} else {
						l.Sugar().Debug("SQS: "+*sqsQueueURLToQueueName(qURL)+" is used (metricsSum=", metricsSum, ")")
					}
				} else {
					l.Sugar().Debug("SQS: " + *sqsQueueURLToQueueName(qURL) + " failed, invalid metric")
				}
			}
			wg.Done()
		}(queueURL)
	}
	wg.Wait()
	return m
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
		l.Sugar().Error("missing params --since")
		l.Sugar().Fatalf(err.Error())
	}
	excludePatten, err := helpers.InitExcludePattern(flags.ExcludePatten)
	if err != nil {
		l.Sugar().Fatalf(err.Error())
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
	l = logger.GetLogger(flags.LogLevel)
	rootArgs = defaultAwsSQSArguments()
	l.Debug("--since", zap.Time("sinceParsed", *rootArgs.UnusedSinceDate), zap.String("since", rootArgs.UnusedSince))
	l.Debug("--queue-prefix", zap.String("AwsSQSQueuePrefix", AwsSQSQueuePrefix))
	l.Debug("--sqs-endpoint", zap.String("AwsSqsEndpoint", AwsSqsEndpoint))
	l.Debug("--cloudwatch-endpoint", zap.String("AwsCloudWatchEndpoint", AwsCloudWatchEndpoint))
	l.Debug("--delete", zap.Bool("delete", rootArgs.enableDelete))
	l.Debug("--no-header", zap.Bool("no-header", rootArgs.noHeader))
	if rootArgs.excludePatten != nil {
		l.Debug("--exclude-patten", zap.String("AwsSnsTopicPrefix", rootArgs.excludePatten.String()))
	}
	if rootArgs.UnusedSince == "" {
		l.Sugar().Fatal("missing --since")
	}
}
func GetSQSMetricDataInput(start *time.Time, end *time.Time, queueURL *string) *cloudwatch.GetMetricDataInput {
	l.Sugar().Debugf("Time: delta period %d min", int64(end.Sub(*start).Minutes()))
	l.Sugar().Debugf("Time: start %s", start.UTC().String())
	l.Sugar().Debugf("Time: end %s", end.UTC().String())
	l.Sugar().Debugf("SQS: queueName %s", *sqsQueueURLToQueueName(queueURL))
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

func awsSQSDelete(queues map[string][]string) map[string][]string {
	m := make(map[string][]string)
	for queue := range queues {
		l.Sugar().Debug("SQS: delete ", queue)
		_, err := sqsSvc.DeleteQueue(&sqs.DeleteQueueInput{QueueUrl: &queue})
		m[queue] = []string{*sqsQueueURLToQueueName(&queue), func(err error) string {
			if err != nil {
				return err.Error()
			}
			return "OK"
		}(err)}
	}
	return m
}
