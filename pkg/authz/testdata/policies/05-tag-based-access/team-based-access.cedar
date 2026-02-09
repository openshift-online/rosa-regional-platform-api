permit(
  ?principal,
  action in [ROSA::Action::"DescribeCluster", ROSA::Action::"UpdateClusterConfig", ROSA::Action::"CreateNodePool", ROSA::Action::"DeleteNodePool", ROSA::Action::"UpdateNodePool"],
  resource
)
when { resource.tags["Team"] == context.principalTags["Team"] };

permit(
  ?principal,
  action in [ROSA::Action::"ListClusters", ROSA::Action::"ListNodePools", ROSA::Action::"ListTagsForResource"],
  resource
);
