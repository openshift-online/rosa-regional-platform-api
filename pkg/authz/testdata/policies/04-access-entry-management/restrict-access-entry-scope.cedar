permit(
  ?principal,
  action in [ROSA::Action::"CreateAccessEntry", ROSA::Action::"UpdateAccessEntry"],
  resource
)
when { context.accessScope == "namespace" };

forbid(
  ?principal,
  action in [ROSA::Action::"CreateAccessEntry", ROSA::Action::"UpdateAccessEntry"],
  resource
)
when { context.accessScope == "cluster" };

permit(
  ?principal,
  action in [ROSA::Action::"DescribeAccessEntry", ROSA::Action::"ListAccessEntries", ROSA::Action::"ListAccessPolicies"],
  resource
);
