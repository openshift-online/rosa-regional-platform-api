permit(
  ?principal,
  action in [ROSA::Action::"CreateAccessEntry", ROSA::Action::"DeleteAccessEntry", ROSA::Action::"DescribeAccessEntry", ROSA::Action::"ListAccessEntries", ROSA::Action::"UpdateAccessEntry"],
  resource == ROSA::Resource::"arn:aws:rosa:us-west-2:111122223333:cluster/my-cluster"
);

permit(
  ?principal,
  action in [ROSA::Action::"DescribeAccessEntry", ROSA::Action::"UpdateAccessEntry", ROSA::Action::"DeleteAccessEntry"],
  resource
)
when { resource like "arn:aws:rosa:us-west-2:111122223333:access-entry/my-cluster/*" };

permit(
  ?principal,
  action == ROSA::Action::"ListAccessPolicies",
  resource
);
