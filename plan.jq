def lpad(n): (" " * (n - (tostring | length))) + tostring;
def predicates: map(select(.type | strings | endswith("Condition") or . == "Split Range"));
.. | .planNodes? | values |
. as $planNodes |
(map(select(.kind == "RELATIONAL") | .index // 0) | max | tostring | length) as $maxRelationalNodeIDLength |
# render tree part
(
  {depth: 0, planNode: .[0]} |
  recurse(
    (.depth + 1) as $depth |
    .planNode.childLinks[] |
    .type as $linkType |
    $planNodes[.childIndex] |
    select(.kind == "RELATIONAL" or $linkType == "Scalar") |
    {planNode: ., $depth, $linkType}
  ) |
  .planNode as $currentNode |
  {
    idStr: (.planNode.index // 0 | tostring | if ($currentNode.childLinks | predicates) != [] then "*\(.)" end | lpad($maxRelationalNodeIDLength + 1)),
    displayNameStr: (
      [
        .planNode.metadata.call_type,
        .planNode.metadata.iterator_type,
        (.planNode.metadata.scan_type | rtrimstr("Scan")),
        .planNode.displayName
      ] | map(values) | join(" ")
    ),
    linkTypeStr: (.linkType | if . then "[\(.)] " else "" end),
    indent: ("  " * .depth // ""),
    metadataStr: (
      .planNode.metadata // {} |
      del(.["subquery_cluster_node", "scan_type", "iterator_type", "call_type"]) |
      to_entries |
      map(if .key == "scan_target" then .key = ($currentNode.metadata.scan_type | rtrimstr("Scan")) end | "\(.key): \(.value)") |
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
    .childLinks // [] | predicates | to_entries[] |
    {
       type: .value.type,
       prefix: (if .key == 0 then "\($nodeIndex // 0):" else "" end),
       description: $planNodes[.value.childIndex].shortRepresentation.description,
    } |
    "\(.prefix | lpad($maxRelationalNodeIDLength + 2)) \(.type): \(.description)"
  ) | select(. != []) | "Predicates:", .[]
)
