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

cat <<EOF | duckdb -json | tee machine_types.json
	SELECT
		distinct(name),
		VCpus as "vcpu",
		memoryGB as "memory",
		acceleratorCount as "gpu",
		acceleratorType as "gpu_type",
	FROM read_csv('https://gcloud-compute.com/machine-types-regions.csv')
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