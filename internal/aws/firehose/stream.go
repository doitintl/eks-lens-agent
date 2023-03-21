package firehose

import (
	"bytes"
	"context"
	"encoding/json"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/firehose"
	"github.com/aws/aws-sdk-go-v2/service/firehose/types"
	_types "github.com/doitintl/eks-lens-agent/internal/usage"
	"github.com/pkg/errors"
)

type Uploader interface {
	Upload(ctx context.Context, records []*_types.Pod) error
}

type firehoseUploader struct {
	Client     *firehose.Client
	StreamName string
}

func NewUploader(ctx context.Context, streamName string) (Uploader, error) {
	// create a new Amazon Kinesis Data Firehose client
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "loading AWS config")
	}
	client := firehose.NewFromConfig(cfg)
	return &firehoseUploader{
		Client:     client,
		StreamName: streamName,
	}, nil
}

// Upload records toAmazon Kinesis Data Firehose using the PutRecordBatch API
// https://docs.aws.amazon.com/firehose/latest/APIReference/API_PutRecordBatch.html
func (u *firehoseUploader) Upload(ctx context.Context, records []*_types.Pod) error {
	// send records to Amazon Kinesis Data Firehose by batches of 500 records
	for i := 0; i < len(records); i += 500 {
		j := i + 500
		if j > len(records) {
			j = len(records)
		}
		// serialize records[i:j] to JSON
		// convert to Compact JSON
		batch := make([]types.Record, 0, j-i)
		for k := i; k < j; k++ {
			buffer, err := json.Marshal(records[k])
			if err != nil {
				return errors.Wrap(err, "marshaling record")
			}
			dst := &bytes.Buffer{}
			err = json.Compact(dst, buffer)
			if err != nil {
				return errors.Wrap(err, "compacting record")
			}
			batch = append(batch, types.Record{
				Data: dst.Bytes(),
			})
		}

		// send records[i:j] to Amazon Kinesis Data Firehose
		input := &firehose.PutRecordBatchInput{
			DeliveryStreamName: aws.String(u.StreamName),
			Records:            batch,
		}
		_, err := u.Client.PutRecordBatch(ctx, input)
		if err != nil {
			return errors.Wrap(err, "putting record batch to Amazon Kinesis Data Firehose")
		}
	}
	return nil
}
