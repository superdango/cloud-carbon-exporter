# WIP: Cloud Carbon Exporter

    go build \
        -o exporter \
        -ldflags="-s -w" \
        github.com/superdango/cloud-carbon-exporter/cmd && ./exporter -cloud.provider=aws -log.level=debug
