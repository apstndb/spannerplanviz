def lpad(n): (" " * (n - (tostring | length))) + tostring;
def ispredicate: .type | strings | endswith("Condition") or . == "Split Range";

.. | .planNodes? | values |
. as $planNodes |
(map(select(.kind == "RELATIONAL") | .index // 0) | max | tostring | length) as $maxRelationalNodeIDLength |
# render tree part
(
  {} |
  recurse(
    {depth: ((.depth // 0) + 1), link: $planNodes[.link.childIndex // 0].childLinks[]};
    select($planNodes[.link.childIndex // 0].kind == "RELATIONAL" or .link.type == "Scalar")
  ) |
  .link.type as $linkType |
  (.depth // 0) as $depth |
  (.link.childIndex // 0) as $index |
  $planNodes[$index] |
  . as $currentNode |
  (.metadata.scan_type | rtrimstr("Scan")) as $scanType |
  {
    idStr: (if $currentNode.childLinks | any(ispredicate) then "*\($index)" else $index end | lpad($maxRelationalNodeIDLength + 1)),
    displayNameStr: (
      [
        .metadata.call_type,
        .metadata.iterator_type,
        $scanType,
        .displayName
      ] | map(values) | join(" ")
    ),
    linkTypeStr: ($linkType | if . then "[\(.)] " else "" end),
    indent: ("  " * $depth // ""),
    metadataStr: (
      .metadata // {} |
      del(.["subquery_cluster_node", "scan_type", "iterator_type", "call_type"]) |
      to_entries |
      map(if .key == "scan_target" then .key = $scanType end | "\(.key): \(.value)") |
      sort |
      join(", ") |
      if . != "" then " (\(.))" end
    )
  } |
  "\(.idStr) \(.indent)\(.linkTypeStr)\(.displayNameStr)\(.metadataStr)"
),
# render predicates part
(
  map(
    .index as $nodeIndex |
    .childLinks // [] | map(select(ispredicate)) | to_entries[] |
    {
       type: .value.type,
       prefix: (if .key == 0 then "\($nodeIndex // 0):" else "" end),
       description: $planNodes[.value.childIndex].shortRepresentation.description,
    } |
    "\(.prefix | lpad($maxRelationalNodeIDLength + 2)) \(.type): \(.description)"
  ) | select(. != []) | "Predicates:", .[]
)
