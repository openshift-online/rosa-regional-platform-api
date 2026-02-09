permit(
  ?principal,
  action == ROSA::Action::"DescribeCluster",
  resource
);

permit(
  ?principal,
  action == ROSA::Action::"ListClusters",
  resource
);
