# Copyright (C) 2025 dangofish.com
#
# This program is free software: you can redistribute it and/or modify
# it under the terms of the GNU Affero General Public License as
# published by the Free Software Foundation, either version 3 of the
# License, or (at your option) any later version.
#
# This program is distributed in the hope that it will be useful,
# but WITHOUT ANY WARRANTY; without even the implied warranty of
# MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
# GNU Affero General Public License for more details.
#
# You should have received a copy of the GNU Affero General Public License
# along with this program.  If not, see <https://www.gnu.org/licenses/>.

# This script parse the official pricing data provided by AWS and extracts
# all instance types and their related technical informations. 

#!/bin/sh
set -o pipefail
set -eu

cat <<EOF | duckdb -json | tee instance_types.json
SELECT 
	instance_type, 
	physical_processor,
	vcpu,
	memory,
	if(ssd_count == '', 0, cast(ssd_count as double)) as ssd_count, 
	if(hdd_count == '', 0, cast(hdd_count as double)) as hdd_count,
	if(ssd_size == '', 0, cast(ssd_size as double)) as ssd_size, 
	if(hdd_size == '', 0, cast(hdd_size as double)) as hdd_size, 
FROM (
	SELECT
		distinct("Instance Type") as "instance_type",
		"Physical Processor" as "physical_processor", 
		cast("vCPU" as DOUBLE) as "vcpu",
		cast(regexp_extract("Memory",'([0-9\.]+)\sGi*B', 1) as DOUBLE) as "memory",
		regexp_extract(if("Storage"=='' or "Storage"=='NA', '0 x 0 GB', "Storage"), '([0-9]+)\s[xX]\s([0-9]+)(.+)SSD$', 1) as ssd_count,
		regexp_extract(if("Storage"=='' or "Storage"=='NA', '0 x 0 GB', "Storage"), '([0-9]+)\s[xX]\s([0-9]+)(.+)SSD$', 2) as ssd_size,
		regexp_extract(if("Storage"=='' or "Storage"=='NA', '0 x 0 GB', "Storage"), '^(?!.*SSD)([0-9]+)\s[xX]\s([0-9]+)', 1) as hdd_count,
		regexp_extract(if("Storage"=='' or "Storage"=='NA', '0 x 0 GB', "Storage"), '^(?!.*SSD)([0-9]+)\s[xX]\s([0-9]+)', 2) as hdd_size,
		cast(coalesce("GPU", 0) AS DOUBLE) AS "gpu",
		cast(regexp_extract(if("GPU Memory"=='NA', '0 GB', "GPU Memory"),'([0-9\.]+)\sGi*B', 1) AS DOUBLE) AS "gpu_memory"
	FROM read_csv('$1')
	WHERE "Product Family" LIKE 'Compute Instance%'
	ORDER BY "Instance Type"
)
EOF

# Regex explanations:
#
# ([0-9]+)\s[xX]\s([0-9]+)(.+)SSD$ matches
# 8 x 1000 SSD
# 1 x 50 NVMe SSD
# 1 x 900 NVMe SSD
# 2 x 120 SSD
# 4 x 3750 NVMe SSD
#
# ^(?!.*SSD)([0-9]+)\s[xX]\s([0-9]+) matches
# 1 x 3750GB
# 4 x 2000 HDD
# 12 x 3750GB
# 8 x 2000 HDD