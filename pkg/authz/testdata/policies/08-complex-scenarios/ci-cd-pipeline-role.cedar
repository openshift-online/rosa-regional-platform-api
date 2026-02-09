permit(
  ?principal,
  action in [ROSA::Action::"DescribeCluster", ROSA::Action::"ListClusters"],
  resource
);

permit(
  ?principal,
  action in [ROSA::Action::"DescribeNodePool", ROSA::Action::"ListNodePools", ROSA::Action::"UpdateNodePool", ROSA::Action::"ScaleNodePool"],
  resource
);

permit(
  ?principal,
  action in [ROSA::Action::"UpdateClusterVersion", ROSA::Action::"ListTagsForResource"],
  resource
);

forbid(
  ?principal,
  action in [ROSA::Action::"CreateCluster", ROSA::Action::"DeleteCluster"],
  resource
);

forbid(
  ?principal,
  action in [ROSA::Action::"CreateAccessEntry", ROSA::Action::"DeleteAccessEntry", ROSA::Action::"UpdateAccessEntry"],
  resource
);

forbid(
  ?principal,
  action in [ROSA::Action::"CreateNodePool", ROSA::Action::"DeleteNodePool"],
  resource
);
