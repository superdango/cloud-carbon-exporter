# Primitives Hypothesis

## Datacenter effectiveness

Cloud data center power usage effectiveness (PUE) can vary, but it typically averages around 1.15.

- https://sustainability.aboutamazon.com/products-services/aws-cloud
- https://www.cloudcarbonfootprint.org/docs/methodology/#power-usage-effectiveness

## Estimating power consumption of server components

### CPU

Processor power consumption is estimated based on their specified Thermal Design Power (TDP), which represents the maximum heat output under typical workloads. We interpolate actual power usage relative to CPU load using the following mapping:

- 0% CPU usage → 12% of TDP
- 10% CPU usage → 32% of TDP
- 50% CPU usage → 75% of TDP
- 100% CPU usage → 102% of TDP

These values are specified in the [Boavista documentation](<(https://doc.api.boavizta.org/Explanations/components/cpu/#model-adaptation-from-tdp)>).

To account for real-world conditions where power usage often exceeds TDP, we increase the estimate by 60%, as demonstrated in [the referenced study](https://www.eatyourbytes.com/fr/cpu-consommation-maximale/)

### RAM

The model accounts 0.38 W/GB as explained in the following analysis: [Estimating AWS EC2 Instances Power Consumption](https://medium.com/teads-engineering/estimating-aws-ec2-instances-power-consumption-c9745e347959).

### Disk

[The Cruseship article](https://cruiseship.cloud/how-much-power-does-a-hard-drive-use/) estimates that SSDs consume between 1 and 5 watts, while HDDs range from 7 to 12 watts. Based on this, the model assumes an average power consumption of 3W for an SSD and 7.5W for an HDD.

## Estimating Embodied carbon emissions

### Disk

- SSDs, the model estimates `0.16 kgCO2eq per GB`. Source: https://hotcarbon.org/assets/2022/pdf/hotcarbon22-tannu.pdf
- HDDs, the model estimates `53.7 kgCO2eq per drive`. Source: https://www.seagate.com/files/www-content/global-citizenship/en-us/docs/seagate-makara-enterprise-hdd-lca-summary-2016-07-29.pdf

For embodied `kgCO2eq/second` value, the model assumes 4 years of exploitation.

### CPU and Memory

We analysed thousands of machine type on boavista api on all supported cloud platforms. We found a simple model based on vCPUs and Memory that work on average and median. By accouting `6.5 kgCO2eq per vCPU` and `3.34 kgCO2eq per GB` of RAM, we get the same results on average.

```
$ cd primitives/data/processors
$ cat README.md
$ # follow csv files generation
$ duckdb
> select avg(embodied) as embodied_avg, avg(vcpu*6.5+memory*3.34) as cal_avg, median(embodied) as embodied_median, median(vcpu*6.5+memory*3.34) as cal_median from read_csv('*_embodied.csv');
┌────────────────────┬────────────────────┬─────────────────┬────────────┐
│    embodied_avg    │      cal_avg       │ embodied_median │ cal_median │
│       double       │       double       │     double      │   double   │
├────────────────────┼────────────────────┼─────────────────┼────────────┤
│ 1872.2938643702932 │ 1877.7335780409026 │           413.0 │     421.76 │
└────────────────────┴────────────────────┴─────────────────┴────────────┘
```

- embodied_avg: the average embodied kgCO2eq per instance of all size without local disks
- cal_avg: the average calculated by the simple formula
- embodied_median: the median embodied kgCO2eq per instance of all size without local disks
- cal_median: the median calculated by the simple formula
