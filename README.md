[![docker](https://github.com/doitintl/eks-lens-agent/workflows/docker/badge.svg)](https://github.com/doitintl/eks-lens-agent/actions?query=workflow%3A"docker") [![Go Report Card](https://goreportcard.com/badge/github.com/doitintl/eks-lens-agent)](https://goreportcard.com/report/github.com/doitintl/eks-lens-agent) ![GitHub all releases](https://img.shields.io/github/downloads/doitintl/eks-lens-agent/total) 

# EKS Lens Agent

The `eks-lens-agent` is a Kubernetes controller that watches for workload and infrastructure change events and sends them to the S3 bucket though Amazon Kinesis Data Firehose. 


## Deployment

Set the following environment variables:

- `AWS_REGION` - AWS region where the Kinesis Data Firehose Delivery Stream is located
- `AWS_ACCOUNT` - AWS account ID where the Kinesis Data Firehose Delivery Stream is located

### Setup AWS Account

Export AWS account ID:

```shell
export AWS_ACCOUNT=$(aws sts get-caller-identity --query Account --output text)
```

Create S3 bucket for storing events:

```shell
aws s3api create-bucket \
    --bucket eks-lens \
    --create-bucket-configuration LocationConstraint=${AWS_REGION}
```

Create the IAM role that grants Kinesis Data Firehose permission to put data into the bucket.
    
```shell
aws iam create-role \
        --role-name eks-lens-agent \
        --assume-role-policy-document file://./schema/iam-role-trust-policy.json
```

Keep the IAM role ARN for later use.

```shell
export FIREHOSE_ROLE_ARN=arn:aws:iam::$AWS_ACCOUNT:role/eks-lens-firehose
```

Create Amazon Glue database for storing events:

```shell
aws glue create-database \
    --database-input "Name=eks-lens, Description=eks-lens, LocationUri=s3://eks-lens/events/, Parameters={}" \
    --region ${AWS_REGION}
```

Create Amazon Glue schema for storing events:

```shell
 aws glue create-schema \
    --schema-name eks-lens \
    --data-format 'AVRO' \
    --compatibility 'BACKWARD' \
    --schema-definition 'file://./schema/schema.json'
```

Create Amazon Glue table for storing events:

```shell
aws glue create-table \
    --database-name eks-lens \
    --table-input "file://./schema/table.json"
```

Keep the Amazon Glue table ARN for later use: `arn:aws:glue:$AWS_REGION:123456789012:table/eks-lens/events`

```shell
export GLUE_TABLE_ARN=arn:aws:glue:$AWS_REGION:$AWS_ACCOUNT:table/eks-lens/events
```

Create Kinesis Data Firehose Delivery Stream with S3 destination and automatic conversion to Parquet format:

```shell
aws firehose create-delivery-stream \
    --delivery-stream-name eks-lens \
    --extended-s3-destination-configuration \
        "RoleARN=$FIREHOSE_ROLE_ARN, \
        BucketARN=arn:aws:s3:::eks-lens, \
        BufferingHints={SizeInMBs=128, IntervalInSeconds=60}, \
        CompressionFormat=UNCOMPRESSED, \
        Prefix=events/, \
        ErrorOutputPrefix=errors/, \
        S3BackupMode=Disabled, \
        CloudWatchLoggingOptions={ \
          Enabled=true, \
          LogGroupName=/aws/kinesisfirehose/eks-lens, \
          LogStreamName=DeliveryStream \
        }, \
        ProcessingConfiguration={Enabled=false}, \
        DataFormatConversionConfiguration={ \
            Enabled=true, \
            InputFormatConfiguration={ \
                Deserializer={ \
                    OpenXJsonSerDe={ \
                        ConvertDotsInJsonKeysToUnderscores=false \
                    } \
                } \
            }, \
            OutputFormatConfiguration={ \
                Serializer={ \
                    ParquetSerDe={} \
                } \
            }, \
            SchemaConfiguration={ \
                RoleARN=$FIREHOSE_ROLE_ARN, \
                DatabaseName=eks-lens, \
                TableName=events, \
                Region=$AWS_REGION, \
                VersionId=LATEST \
            } \
        }" \
    --region $AWS_REGION
```

Keep the Kinesis Data Firehose Delivery Stream ARN for later use.

```shell
export FIREHOSE_ARN=arn:aws:firehose:$AWS_REGION:$AWS_ACCOUNT:deliverystream/eks-lens
```

Create IAM Policy document that will be used by the `eks-lens-agent` to push events to Kinesis Data Firehose:

```bash
cat <<EOF > schema/iam-role-policy.json
{
    "Version": "2012-10-17",
    "Statement": [
        {
            "Sid": "FirehoseAccess",
            "Effect": "Allow",
            "Action": [
                "firehose:DeleteDeliveryStream",
                "firehose:PutRecord",
                "firehose:PutRecordBatch",
                "firehose:UpdateDestination"
            ],
            "Resource": "$FIREHOSE_ARN"
        },
        {
            "Sid": "GlueAccess",
            "Effect": "Allow",
             "Action": [
                "glue:GetTable",
                "glue:GetTableVersion",
                "glue:GetTableVersions",
                "glue:GetSchema",
				"glue:GetSchemaVersion"
            ],
            "Resource": [
              "arn:aws:glue:$AWS_REGION:$AWS_ACCOUNT:database/eks-lens",
              "arn:aws:glue:$AWS_REGION:$AWS_ACCOUNT:catalog",
              "arn:aws:glue:$AWS_REGION:$AWS_ACCOUNT:table/eks-lens/events",
              "arn:aws:glue:$AWS_REGION:$AWS_ACCOUNT:schema/default-registry/eks-lens"
            ]
        },
        {
            "Sid": "S3Access",
            "Effect": "Allow",      
            "Action": [
                "s3:AbortMultipartUpload",
                "s3:GetBucketLocation",
                "s3:GetObject",
                "s3:ListBucket",
                "s3:ListBucketMultipartUploads",
                "s3:PutObject"
            ],      
            "Resource": [        
                "arn:aws:s3:::eks-lens",
                "arn:aws:s3:::eks-lens/*"		    
            ]
        }
    ]
} 
EOF
```

Create IAM Policy that will be used by the `eks-lens-agent` to push events to Kinesis Data Firehose:

```shell
aws iam create-policy \
    --policy-name eks-lens-agent \
    --policy-document file://./schema/iam-role-policy.json
```

Create IAM role that will be used by the `eks-lens-agent` to push events to Kinesis Data Firehose:

```shell
aws iam attach-role-policy \
    --role-name eks-lens-agent \
    --policy-arn arn:aws:iam::$AWS_ACCOUNT:policy/eks-lens-agent
```

### Configure ServiceAccount

Create a new ServiceAccount, ClusterRole and ClusterRoleBinding:

```shell
kubectl apply -f deploy/rbac.yaml
```

Annotate the ServiceAccount with the IAM role ARN:

```shell
kubectl annotate serviceaccount eks-lens-agent eks.amazonaws.com/role-arn=arn:aws:iam::$AWS_ACCOUNT:role/eks-lens-agent --namespace eks-lens
```

### Deploy eks-lens-agent

Deploy the `eks-lens-agent` to the cluster:

```shell
kubectl apply -f deploy/deployment.yaml
```

## How to build

Run the following command to build the `eks-lens-agent` binary:

```shell
make
```

### Build Docker image

Use Docker `buildx` plugin to build multi-architecture Docker image.

```shell
docker buildx build --platform=linux/arm64,linux/amd64 -t eks-lens-agent -f Dockerfile .
```

## CI/CD

GitHub Actions are used for CI/CD. The following workflows are defined:

- `docker` - builds and pushes Docker image to GitHub Container Registry
- `release` - creates a new GitHub release with changelog and binaries
- `test` - runs linters and unit tests

### Required GitHub secrets

Please specify the following GitHub secret:

- `GITHUB_TOKEN` - GitHub Personal Access Token (with `write/read` packages permission)

