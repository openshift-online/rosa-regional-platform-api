permit(
  ?principal,
  action == ROSA::Action::"DescribeCluster",
  resource
);

permit(
  ?principal,
  action == ROSA::Action::"DescribeNodePool",
  resource
);

permit(
  ?principal,
  action == ROSA::Action::"DescribeAccessEntry",
  resource
);

permit(
  ?principal,
  action == ROSA::Action::"ListClusters",
  resource
);

permit(
  ?principal,
  action == ROSA::Action::"ListNodePools",
  resource
);

permit(
  ?principal,
  action == ROSA::Action::"ListAccessEntries",
  resource
);

permit(
  ?principal,
  action == ROSA::Action::"ListAccessPolicies",
  resource
);

permit(
  ?principal,
  action == ROSA::Action::"ListTagsForResource",
  resource
);
