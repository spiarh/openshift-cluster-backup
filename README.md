# OpenShift Cluster Backup

See [kustomize manifests](./kustomize) to deploy it.

The pod has **to run within the host network** to use the same hostname
as the node and for performance reasons.

The required minimum environment variables to export are the following:

* AWS_ACCESS_KEY_ID
* AWS_SECRET_ACCESS_KEY
* BUCKET_REGION
* BUCKET_NAME

It is not a hard requirement but AWS_SECRET_TOKEN can be exported as well.

**Create an AWS S3 bucket**

```bash
BUCKET="my-super-backup-bucket"
REGION="eu-central-1"
IAM_USER="my-super-backup-bucket"

aws s3api create-bucket \
    --bucket $BUCKET \
    --region $REGION \
    --create-bucket-configuration LocationConstraint=$REGION

aws s3api put-bucket-encryption \
    --bucket $BUCKET \
    --server-side-encryption-configuration '{"Rules": [{"ApplyServerSideEncryptionByDefault": {"SSEAlgorithm": "AES256"}}]}'

aws iam create-user --user-name $IAM_USER

cat >aws-s3-uploads-policy.json<<EOF
{
    "Version": "2012-10-17",
    "Statement": [
        {
            "Effect": "Allow",
            "Action": [
                "s3:Get*",
                "s3:List*",
                "s3:Put*"
            ],
            "Resource": "*"
        }
    ]
}
EOF

aws iam create-policy --policy-name $BUCKET --policy-document file://aws-s3-uploads-policy.json

--> Get Arn from the returned json or just replace account ID

aws iam attach-user-policy --policy-arn arn:aws:iam::<account_id>:policy/$BUCKET --user-name $IAM_USER

aws iam create-access-key --user-name $IAM_USER
```

**List s3 buckets with a specific tag**

It is a bit ugly but there is not native way to do this with the CLI.

`/hack/list-buckets-with-tag-value.sh` allows you to do that.


```bash
$ ./hack/list-buckets-with-tag.sh cluster-backup
>>> Bucket: cluster-b-da0dd49a-de49-4e4f-9f9a-cb248b88df7d
{"ObjectBucketClaim_UID":"12bf918a-ad3b-463f-aba8-aca1133d0b46"}
{"DisasterRecovery":"True"}
{"Provisioner":"aws-s3.io/bucket"}
{"Cluster":"api.example.com"}
{"Namespace":"backup"}
{"ServicePhase":"Prod"}
{"Name":"cluster-backup"}
{"objectbucket.io/reclaimPolicy":"Retain"}
[...]
```
