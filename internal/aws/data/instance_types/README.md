# Instance Types

## Update Instance Type data

This directory contains the script that generates and refresh instance_types.json used by the exporter in order
to determine the number of CPU, RAM and Processor Architecture of all AWS instance types.

The script parses the (4GB+) file shared by AWS to extract a list of instance types with their corresponding infos.

**Prerequisite**

* `DuckDB cli`
* `jq`

```
$ ./instance_types_update.sh https://pricing.us-east-1.amazonaws.com/offers/v1.0/aws/AmazonEC2/current/index.csv
```
or

```
$ curl -o index.csv https://pricing.us-east-1.amazonaws.com/offers/v1.0/aws/AmazonEC2/current/index.csv
$ ./instance_types_update.sh index.csv
```