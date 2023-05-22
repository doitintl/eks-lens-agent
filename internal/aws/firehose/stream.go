package firehose

import (
	"bytes"
	"context"
	"encoding/json"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/firehose"
	"github.com/aws/aws-sdk-go-v2/service/firehose/types"
	"github.com/doitintl/eks-lens-agent/internal/usage"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

type contextKey string

const (
	maxBatchSize              = 500
	developModeKey contextKey = "develop-mode"
)

type Uploader interface {
	Upload(ctx context.Context, records []*usage.PodInfo) error
}

type firehoseUploader struct {
	client *firehose.Client
	log    *logrus.Entry
	stream string
}

func NewUploader(ctx context.Context, log *logrus.Entry, streamName string) (Uploader, error) {
	// create a new Amazon Kinesis Data Firehose client
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "loading AWS config")
	}
	client := firehose.NewFromConfig(cfg)
	return &firehoseUploader{
		client: client,
		log:    log,
		stream: streamName,
	}, nil
}

// Upload records toAmazon Kinesis Data Firehose using the PutRecordBatch API
// https://docs.aws.amazon.com/firehose/latest/APIReference/API_PutRecordBatch.html
func (u *firehoseUploader) Upload(ctx context.Context, records []*usage.PodInfo) error {
	// send records to Amazon Kinesis Data Firehose by batches of 500 records
	for i := 0; i < len(records); i += maxBatchSize {
		j := i + maxBatchSize
		if j > len(records) {
			j = len(records)
		}
		// serialize records[i:j] to JSON
		// convert to Compact JSON
		batch := make([]types.Record, 0, j-i)
		for k := i; k < j; k++ {
			buffer, err := json.Marshal(records[k])
			if err != nil {
				u.log.WithField("pod", records[k].Name).WithError(err).Error("marshaling record")
				continue
			}
			dst := &bytes.Buffer{}
			err = json.Compact(dst, buffer)
			if err != nil {
				u.log.WithField("pod", records[k].Name).WithError(err).Error("compacting record")
				continue
			}
			batch = append(batch, types.Record{
				Data: dst.Bytes(),
			})
		}

		// get develop-mode flag from context
		developMode := false
		if val := ctx.Value(developModeKey); val != nil {
			developMode = val.(bool)
		}

		// send records[i:j] to Amazon Kinesis Data Firehose, if not in develop-mode
		if developMode {
			input := &firehose.PutRecordBatchInput{
				DeliveryStreamName: aws.String(u.stream),
				Records:            batch,
			}
			_, err := u.client.PutRecordBatch(ctx, input)
			if err != nil {
				return errors.Wrap(err, "putting record batch to Amazon Kinesis Data Firehose")
			}
		}
	}
	return nil
}
