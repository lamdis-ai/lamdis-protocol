#!/bin/sh
# Publishes the www->apex 301 CloudFront Function and attaches it to lamdis.ai's default behavior.
set -e
export AWS_PROFILE=aws-admin
cd "$(dirname "$0")"
ET=$(aws cloudfront describe-function --name lamdis-landing-canonical-host --stage DEVELOPMENT --query ETag --output text)
ARN=$(aws cloudfront publish-function --name lamdis-landing-canonical-host --if-match "$ET" --query 'FunctionSummary.FunctionMetadata.FunctionARN' --output text)
aws cloudfront get-distribution-config --id E2M2N52W2HHFMX > dist.json
python3 -c "
import json,sys
d=json.load(open('dist.json'));c=d['DistributionConfig']
c['DefaultCacheBehavior']['FunctionAssociations']={'Quantity':1,'Items':[{'FunctionARN':'$ARN','EventType':'viewer-request'}]}
json.dump(c,open('distcfg.json','w'));open('etag.txt','w').write(d['ETag'])"
aws cloudfront update-distribution --id E2M2N52W2HHFMX --if-match "$(cat etag.txt)" --distribution-config file://distcfg.json --query Distribution.Status --output text
