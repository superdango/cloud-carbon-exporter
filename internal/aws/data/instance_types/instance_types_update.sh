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

#!/bin/sh

set -o pipefail
set -eu

cat <<EOF | duckdb -json | jq . | tee instance_types.json
SELECT
	distinct("Instance Type") as "instance_type",
	"Physical Processor" as "physical_processor", 
	cast("vCPU" as DOUBLE) as "vcpu",
	cast(regexp_extract("Memory",'([0-9\.]+)\sGiB', 1) as DOUBLE) as "memory"
FROM read_csv('$1') 
WHERE "Product Family" LIKE 'Compute Instance%'
ORDER BY "Instance Type"
EOF