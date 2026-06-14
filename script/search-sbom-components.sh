#!/usr/bin/env bash
set -euo pipefail

DIR="${1:-sboms}"
KEY="${2:?Usage: $0 [SBOM_DIR] <KEY>}"

found=0

echo -e "namespace\tcomponent.name\tcomponent.version\tcomponent.purl\tsbom"

while IFS= read -r -d '' sbom; do
  rel="${sbom#"$DIR"/}"
  namespace="${rel%%/*}"
  component_name="$(jq -r '.metadata.component.name // ""' "$sbom")"
  
  result="$(
    sbom-utility query \
      -i "$sbom" \
      --from components \
      -q |
    jq -r \
      --arg key "$KEY" \
      --arg namespace "$namespace" \
      --arg component_name "$component_name" \
      --arg file "$sbom" '
      .[]
      | select(
          ((.name // "") | ascii_downcase | contains($key | ascii_downcase))
          or
          ((.purl // "") | ascii_downcase | contains($key | ascii_downcase))
        )
      | [
          $namespace,
          $component_name,
          (.name // ""),
          (.version // ""),
          (.purl // ""),
          $file
        ]
      | @tsv
    '
  )"

  if [[ -n "$result" ]]; then
    found=1
    echo "$result"
  fi
done < <(
  find "$DIR" \
    -mindepth 2 \
    -type f \
    -name '*.json' \
    -print0
)

exit "$(( found == 1 ? 0 : 1 ))"
