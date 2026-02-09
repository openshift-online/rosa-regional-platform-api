permit(
  ?principal,
  action in [ROSA::Action::"DescribeCluster", ROSA::Action::"DescribeNodePool", ROSA::Action::"ListClusters", ROSA::Action::"ListNodePools"],
  resource
);

permit(
  ?principal,
  action == ROSA::Action::"UpdateNodePool",
  resource
)
when { resource like "arn:aws:rosa:*:111122223333:nodepool/*" };
