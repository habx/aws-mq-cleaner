# aws-mq-cleaner

[![codecov](https://codecov.io/gh/habx/aws-mq-cleaner/branch/dev/graph/badge.svg?token=376464GH1H)](https://codecov.io/gh/habx/aws-mq-cleaner)
[![Release](https://img.shields.io/github/v/release/habx/aws-mq-cleaner)](https://github.com/habx/aws-mq-cleaner/releases/latest)
[![Go version](https://img.shields.io/github/go-mod/go-version/habx/aws-mq-cleaner/dev)](https://golang.org/doc/devel/release.html)
[![Docker](https://img.shields.io/docker/pulls/habx/aws-mq-cleaner)](https://hub.docker.com/r/habx/aws-mq-cleaner)
[![CircleCI](https://img.shields.io/circleci/build/github/habx/aws-mq-cleaner/dev)](https://app.circleci.com/pipelines/github/habx/aws-mq-cleaner)
[![License](https://img.shields.io/github/license/habx/aws-mq-cleaner)](/LICENSE)

## About

`aws-mq-cleaner` is a tool to remove unused SQS queues and SNS topics.

### Deletion method

#### SQS

We're getting the metrics from Cloudwatch.
If `NumberOfMessagesReceived` and `NumberOfEmptyReceipts` are set to zero, we're considering the queue to be unused.

#### SNS

If `subscriptionsConfirmed` and `subscriptionsPending` are at zero, we're considering the topic to be unused.

> **_NOTE:_** You can add a tag to read an "update date" to prevent a topic without subscriptions to be deleted. The tag name is configurable, you can define it via the [Docs](docs/aws-mq-cleaner.md). The tag format shall be in [ISO-8601](https://en.wikipedia.org/wiki/ISO_8601).


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
