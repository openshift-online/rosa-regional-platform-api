permit(
  ?principal,
  action in [ROSA::Action::"CreateNodePool", ROSA::Action::"ListNodePools", ROSA::Action::"TagResource", ROSA::Action::"UntagResource"],
  resource == ROSA::Resource::"arn:aws:rosa:us-west-2:111122223333:cluster/prod-cluster"
);

permit(
  ?principal,
  action in [ROSA::Action::"DeleteNodePool", ROSA::Action::"DescribeNodePool", ROSA::Action::"UpdateNodePool", ROSA::Action::"TagResource", ROSA::Action::"UntagResource"],
  resource
)
when { resource like "arn:aws:rosa:us-west-2:111122223333:nodepool/prod-cluster/*" };
