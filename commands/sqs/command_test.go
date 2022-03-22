package sqs_test

import (
	"strconv"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/cloudwatch"
	"github.com/aws/aws-sdk-go/service/sqs"
	"github.com/habx/aws-mq-cleaner/commands"
	here "github.com/habx/aws-mq-cleaner/commands/sqs"
	"github.com/habx/aws-mq-cleaner/helpers"
	. "github.com/smartystreets/goconvey/convey"
)

func Test_SQS(t *testing.T) {
	sqsLocalStack := helpers.GetEnv("TEST_SQS_ENDPOINT", "http://localhost.localstack.cloud:4566")
	cloudWatchLocalStack := helpers.GetEnv("TEST_CLOUDWATCH_ENDPOINT", "http://localhost.localstack.cloud:4566")
	queue := CreateQueue(sqsLocalStack)
	queue = CreateQueue(sqsLocalStack)
	CreateMetrics(cloudWatchLocalStack, queue.QueueUrl)
	queue = CreateQueueWithOldDate(sqsLocalStack)
	CreateMetrics(cloudWatchLocalStack, queue.QueueUrl)
	Convey("sqs: std cmd ", t, func() {
		args := []string{"--loglevel=debug", "sqs", "--sqs-endpoint=" + sqsLocalStack, "--cloudwatch-endpoint=" + cloudWatchLocalStack}
		commands.RootCommand.SetArgs(args)
		err := commands.RootCommand.Execute()
		So(err, ShouldBeNil)
	})
	Convey("sqs: queue prefix test", t, func() {
		args := []string{"--loglevel=debug", "sqs", "--sqs-endpoint=" + sqsLocalStack, "--cloudwatch-endpoint=" + cloudWatchLocalStack, "--queue-prefix=test"}
		commands.RootCommand.SetArgs(args)
		err := commands.RootCommand.Execute()
		So(err, ShouldBeNil)
	})
	Convey("sqs: since 1d", t, func() {
		args := []string{"--loglevel=debug", "sqs", "--sqs-endpoint=" + sqsLocalStack, "--cloudwatch-endpoint=" + cloudWatchLocalStack, "--since=1d"}
		commands.RootCommand.SetArgs(args)
		err := commands.RootCommand.Execute()
		So(err, ShouldBeNil)
	})

	Convey("sqs: since 1d and check tags", t, func() {
		args := []string{"--loglevel=debug", "sqs", "--sqs-endpoint=" + sqsLocalStack, "--cloudwatch-endpoint=" + cloudWatchLocalStack, "--since=1d", "--check-tag-name-update-date=update_date"}
		commands.RootCommand.SetArgs(args)
		err := commands.RootCommand.Execute()
		So(err, ShouldBeNil)
	})

	Convey("sqs: enable delete", t, func() {
		args := []string{"--loglevel=debug", "sqs", "--sqs-endpoint=" + sqsLocalStack, "--cloudwatch-endpoint=" + cloudWatchLocalStack, "-d", "--no-header=true"}
		commands.RootCommand.SetArgs(args)
		err := commands.RootCommand.Execute()
		So(err, ShouldBeNil)
	})
	queue = CreateQueue(sqsLocalStack)
	CreateMetrics(cloudWatchLocalStack, queue.QueueUrl)
	Convey("sqs: queue exclude pattern test", t, func() {
		args := []string{"--loglevel=debug", "--exclude-patten=test*", "sqs", "--sqs-endpoint=" + sqsLocalStack, "--cloudwatch-endpoint=" + cloudWatchLocalStack, "--queue-prefix=test"}
		commands.RootCommand.SetArgs(args)
		err := commands.RootCommand.Execute()
		So(err, ShouldBeNil)
	})
	queue = CreateQueue(sqsLocalStack)
	queue = CreateQueueWithOldDate(sqsLocalStack)
	CreateMetrics(cloudWatchLocalStack, queue.QueueUrl)
	queue = CreateQueue(sqsLocalStack)
	CreateMetrics(cloudWatchLocalStack, queue.QueueUrl)
	Convey("sqs: enable delete with invalid metric", t, func() {
		args := []string{"--loglevel=debug", "--exclude-patten=xx", "sqs", "--sqs-endpoint=" + sqsLocalStack, "--cloudwatch-endpoint=" + cloudWatchLocalStack, "-d=true", "--no-header=false", "--since=10d"}
		commands.RootCommand.SetArgs(args)
		err := commands.RootCommand.Execute()
		So(err, ShouldBeNil)
	})
}
func CreateQueue(endpoint string) *sqs.CreateQueueOutput {
	sqsSvc := sqs.New(helpers.GetAwsSession(endpoint))
	queueOutput, err := sqsSvc.CreateQueue(&sqs.CreateQueueInput{QueueName: aws.String("testing"),
		Tags: map[string]*string{
			"update_date": aws.String(time.Now().Format("2006-01-02T15:04:05.000-03:00")),
		}})
	if err != nil {
		panic(err)
	}
	return queueOutput
}
func CreateQueueWithOldDate(endpoint string) *sqs.CreateQueueOutput {
	sqsSvc := sqs.New(helpers.GetAwsSession(endpoint))
	queueOutput, err := sqsSvc.CreateQueue(&sqs.CreateQueueInput{QueueName: aws.String("testing-" + strconv.Itoa(int(time.Now().Unix()))),
		Tags: map[string]*string{
			"update_date": aws.String("2020-01-02T15:04:05.000-03:00"),
		}})
	if err != nil {
		panic(err)
	}
	return queueOutput
}
func CreateMetrics(endpoint string, queueURL *string) {
	cwSvc := cloudwatch.New(helpers.GetAwsSession(endpoint))
	aws.String("2021-01-24T00:00:00.000-03:00")
	now := time.Now().UTC()
	now10 := now.Add(10 * time.Minute)

	x := here.GetSQSMetricDataInput(
		&now,
		&now10,
		queueURL,
	)
	var metricsList []*cloudwatch.MetricDatum
	for _, query := range x.MetricDataQueries {

		new := cloudwatch.MetricDatum{
			Dimensions: query.MetricStat.Metric.Dimensions,
			MetricName: query.MetricStat.Metric.MetricName,
			Timestamp:  aws.Time(now.Add(10 - time.Minute)),
			Unit:       aws.String("Count"),
			Value:      aws.Float64(0),
			Counts: []*float64{
				aws.Float64(1),
			},
		}
		new.StorageResolution = aws.Int64(1)
		metricsList = append(metricsList, &new)
	}
	_, err := cwSvc.PutMetricData(&cloudwatch.PutMetricDataInput{
		MetricData: metricsList,
		Namespace:  aws.String("AWS/SQS"),
	})
	if err != nil {
		panic(err)
	}
}

//func CreateInvalidMetrics(endpoint string, queueURL *string) {
//	cwSvc := cloudwatch.New(helpers.GetAwsSession(endpoint))
//	aws.String("2021-01-24T00:00:00.000-03:00")
//	now := time.Now().UTC()
//	x := here.GetSQSMetricDataInput(
//		&now,
//		&now,
//		queueURL,
//	)
//
//	var metricsList []*cloudwatch.MetricDatum
//	for _, query := range x.MetricDataQueries {
//		new := cloudwatch.MetricDatum{
//			Dimensions:        query.MetricStat.Metric.Dimensions,
//			MetricName:        query.MetricStat.Metric.MetricName,
//			Timestamp:         &now,
//			Unit:              aws.String("Count"),
//			Value:             nil,
//		}
//		metricsList = append(metricsList, &new)
//	}
//	_, err := cwSvc.PutMetricData(&cloudwatch.PutMetricDataInput{
//		MetricData: metricsList,
//		Namespace:  aws.String("AWS/SQS"),
//	})
//	if err != nil {
//		panic(err)
//	}
//}
