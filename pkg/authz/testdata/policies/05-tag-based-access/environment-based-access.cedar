permit(
  ?principal,
  action,
  resource
)
when { resource.tags["Environment"] == "development" };

permit(
  ?principal,
  action in [ROSA::Action::"DescribeCluster", ROSA::Action::"DescribeNodePool", ROSA::Action::"DescribeAccessEntry", ROSA::Action::"ListClusters", ROSA::Action::"ListNodePools", ROSA::Action::"ListAccessEntries", ROSA::Action::"ListAccessPolicies", ROSA::Action::"ListTagsForResource"],
  resource
)
when { resource.tags["Environment"] == "production" };

permit(
  ?principal,
  action in [ROSA::Action::"ListClusters", ROSA::Action::"ListAccessPolicies"],
  resource
);
