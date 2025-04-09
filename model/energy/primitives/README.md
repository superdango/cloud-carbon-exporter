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
