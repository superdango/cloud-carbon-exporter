# Cloud Carbon Exporter

> This work is currently in progress. Do not use before the official release announcement around May 2025. Nothing stable, data is wrong, everything broken, don't contribute yet. Thanks !

Your cloud energy draw and carbon emissions in realtime.

The exporter enables your Cloud team to adhere in [Carbon Driven Development](#) principles.

## How it works

This exporter will discover all resources running in a specified Cloud project or account and estimate the energy ⚡ (watt) used by them and calculate the associated CO₂eq emissions ☁️ based on their location.

```mermaid
      sequenceDiagram
            Prometheus->>cloud-carbon-exporter: scrape metrics
            cloud-carbon-exporter->>AWS Cost Explorer API: query used services and regions
            cloud-carbon-exporter->>(us-west-1) EC2 Instance API: Describe Instances
            cloud-carbon-exporter->>(eu-west-3) S3 API: Describe Buckets
            cloud-carbon-exporter->>(eu-west-3) Cloudwatch API: Get instances statistics
            cloud-carbon-exporter->>(eu-west-3) Cloudwatch API: Get Buckets statistics
            cloud-carbon-exporter->>Prometheus: Returns Watts and CO2 metrics
```

### Estimated Watts

Each resource discovered by the exporter is embellished with additional data from specific apis or cloud monitoring. Those health signals are used by a calculation model to precisely estimate the current energy usage (CPU load, Storage used, requests/seconds, etc.)

### Estimated CO₂eq/second

Once the watt estimation is complete, we match the resource's location with a carbon coefficient to calculate the CO₂ equivalent (CO₂eq).

The model is based on various [public data shared by cloud providers.](https://github.com/GoogleCloudPlatform/region-carbon-info).

### Demo

You can find a demo grafana instance on : [https://demo.carbondriven.dev](https://demo.carbondriven.dev/public-dashboards/04a3c6d5961c4463b91a3333d488e584)

## Install

You can download the official Docker Image on the [Github Package Registry](https://github.com/superdango/cloud-carbon-exporter/pkgs/container/cloud-carbon-exporter)

```
$ docker pull ghcr.io/superdango/cloud-carbon-exporter:latest
```

## Configuration

The Cloud Carbon Exporter can work on Google Cloud Platform, Amazon Web Service and Scaleway (more to come)

### Google Cloud Platform

The exporter uses GCP Application Default Credentials:

- GOOGLE_APPLICATION_CREDENTIALS environment variable
- `gcloud auth application-default` login command
- The attached service account, returned by the metadata server (inside GCP)

```
$ docker run -p 2922 ghcr.io/superdango/cloud-carbon-exporter:latest \
        -cloud.provider=gcp \
        -gcp.projectid=myproject
```

### Amazon Web Services

Configure the exporter via:

- Environment Variables (AWS_SECRET_ACCESS_KEY, AWS_ACCESS_KEY_ID, AWS_SESSION_TOKEN)
- Shared Configuration
- Shared Credentials files.

```
$ docker run -p 2922 ghcr.io/superdango/cloud-carbon-exporter:latest \
        -cloud.provider=aws
```

### Scaleway

Configure the exporter via:

- Environment Variables (SCW_ACCESS_KEY, SCW_SECRET_KEY)

```
$ docker run -p 2922 ghcr.io/superdango/cloud-carbon-exporter:latest \
        -cloud.provider=scw
```

### Deployment

Cloud Carbon Exporter can easily run on serverless platform like GCP Cloud Run or AWS Lambda.

### Usage

```
Usage of ./cloud-carbon-exporter:
  -cloud.aws.defaultregion string
        aws default region (default "us-east-1")
  -cloud.aws.rolearn string
        aws role arn to assume
  -cloud.gcp.projectid string
        gcp project to explore resources from
  -cloud.provider string
        cloud provider type (gcp, aws, scw)
  -demo.enabled string
        return fictive demo data (default "false")
  -listen string
        addr to listen to (default "0.0.0.0:2922")
  -log.format string
        log format (text, json) (default "text")
  -log.level string
        log severity (debug, info, warn, error) (default "info")

Environment Variables:
  SCW_ACCESS_KEY
        scaleway access key
  SCW_SECRET_KEY
```

## Development

    go build \
        -o exporter \
        github.com/superdango/cloud-carbon-exporter/cmd && \
        ./exporter -cloud.provider=aws -log.level=debug

## Licence

This software is provided as is, without waranty under [AGPL 3.0 licence](https://www.gnu.org/licenses/agpl-3.0.en.html)

## Acknowledgements

Many of the most valuable contributions are in the forms of testing, feedback, and documentation. These help harden software and streamline usage for other users.

We want to give special thanks to individuals who invest much of their time and energy into the project to help make it better:

- Thanks to [Hakim Rouatbi](https://github.com/hakro), [Raphaël Cosperec](https://github.com/rcosperec) for giving early AWS expertise feedback.

## ⭐ Sponsor

[dangofish.com](dangofish.com) - Tools and Services for Cloud Carbon Developers.
