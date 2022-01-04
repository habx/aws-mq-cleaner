# aws-mq-cleaner

[![codecov](https://codecov.io/gh/habx/aws-mq-cleaner/branch/dev/graph/badge.svg?token=376464GH1H)](https://codecov.io/gh/habx/aws-mq-cleaner)
[![Release](https://img.shields.io/github/v/release/habx/aws-mq-cleaner)](https://github.com/habx/aws-mq-cleaner/releases/latest)
[![Go version](https://img.shields.io/github/go-mod/go-version/habx/aws-mq-cleaner/dev)](https://golang.org/doc/devel/release.html)
[![Docker](https://img.shields.io/docker/pulls/habx/aws-mq-cleaner)](https://hub.docker.com/r/habx/aws-mq-cleaner)
[![CircleCI](https://img.shields.io/circleci/build/github/habx/aws-mq-cleaner/dev)](https://app.circleci.com/pipelines/github/habx/aws-mq-cleaner)
[![License](https://img.shields.io/github/license/habx/aws-mq-cleaner)](/LICENSE)

## About

`aws-mq-cleaner` is a tool to remove sqs queues and sns topics that are no more used.

**Deletion method** : 

* SQS

I'm getting the metrics from cloudwatch.
If `NumberOfMessagesReceived` and `NumberOfEmptyReceipts` are at zero I consider the queue to not be used anymore.

* SNS

If `subscriptionsConfirmed` and `subscriptionsPending` are at zero I consider the topic to be more used.

> **_NOTE:_** You can add a tag to read an "update date" to define if you should delete the topic or the queue. This tag is configurable, you can define it via the [Docs](docs/aws-mq-cleaner.md)


## Install

### Docker

```bash
docker pull habx/aws-mq-cleaner
docker container run --rm habx/aws-mq-cleaner --help
```

### Binary

#### MACOS

Set VERSION

```bash
VERSION=vx.x.x wget https://github.com/habx/aws-mq-cleaner/releases/download/${VERSION}/aws-mq-cleaner_darwin_amd64.gz
```

#### LINUX

```bash
VERSION=vx.x.x wget https://github.com/habx/aws-mq-cleaner/releases/download/${VERSION}/aws-mq-cleaner_linux_amd64.gz
```

### go source

```bash
go get -t github.com/habx/aws-mq-cleaner
aws-mq-cleaner --help
```

## Docs

[Docs](docs/aws-mq-cleaner.md)
