# Cloud Hypothesis

## Estimating power consumption

### Block storage

According to a [Google Cloud blog post](https://cloud.google.com/blog/products/compute/high-durability-persistent-disk) on persistent disks, durability is always equal to or greater than that of three-way replication. Data blocks are distributed across multiple physical disks and storage-dedicated racks. As result, single physical disk typically hosts multiple virtual disks.

In this model, a block disk is assumed to represent a fraction of three 2 TB physical disks, with an additional 10% added to account for rack overhead.

For example, a 1 TB block disk is estimated to consume the equivalent of 1.5 physical disks (3 × 1 TB) plus 10% overhead.
