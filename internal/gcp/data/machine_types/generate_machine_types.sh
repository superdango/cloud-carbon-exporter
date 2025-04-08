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
	ORDER BY name
EOF
