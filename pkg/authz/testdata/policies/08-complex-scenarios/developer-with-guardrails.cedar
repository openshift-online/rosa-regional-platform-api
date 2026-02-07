permit(
  ?principal,
  action in [ROSA::Action::"DescribeCluster", ROSA::Action::"DescribeNodePool", ROSA::Action::"DescribeAccessEntry", ROSA::Action::"ListClusters", ROSA::Action::"ListNodePools", ROSA::Action::"ListAccessEntries", ROSA::Action::"ListTagsForResource"],
  resource
);

permit(
  ?principal,
  action,
  resource
)
when { resource.tags["Environment"] == "development" || resource.tags["Environment"] == "staging" };

permit(
  ?principal,
  action == ROSA::Action::"CreateCluster",
  resource
)
when { context.requestTags["Owner"] == context.username };

forbid(
  ?principal,
  action == ROSA::Action::"CreateCluster",
  resource
)
when { context.requestTags["Environment"] == "production" };

forbid(
  ?principal,
  action in [ROSA::Action::"DeleteCluster", ROSA::Action::"DeleteNodePool", ROSA::Action::"UpdateClusterConfig", ROSA::Action::"UpdateClusterVersion", ROSA::Action::"UpdateNodePool", ROSA::Action::"ScaleNodePool", ROSA::Action::"CreateAccessEntry", ROSA::Action::"DeleteAccessEntry"],
  resource
)
when { resource.tags["Environment"] == "production" };

forbid(
  ?principal,
  action in [ROSA::Action::"TagResource", ROSA::Action::"UntagResource"],
  resource
)
when { context.tagKeys.contains("Environment") || context.tagKeys.contains("Owner") };

forbid(
  ?principal,
  action == ROSA::Action::"UpdateAccessEntry",
  resource
)
when { context.policyArn like "*ROSAClusterAdminPolicy" };
