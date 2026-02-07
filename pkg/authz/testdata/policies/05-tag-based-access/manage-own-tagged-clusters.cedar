permit(
  ?principal,
  action == ROSA::Action::"CreateCluster",
  resource
)
when { context.requestTags["Owner"] == context.username };

permit(
  ?principal,
  action in [ROSA::Action::"DeleteCluster", ROSA::Action::"UpdateClusterConfig", ROSA::Action::"UpdateClusterVersion", ROSA::Action::"DescribeCluster", ROSA::Action::"CreateNodePool", ROSA::Action::"DeleteNodePool", ROSA::Action::"UpdateNodePool"],
  resource
)
when { resource.tags["Owner"] == context.username };

permit(
  ?principal,
  action in [ROSA::Action::"ListClusters", ROSA::Action::"ListNodePools", ROSA::Action::"DescribeNodePool", ROSA::Action::"ListTagsForResource"],
  resource
);
