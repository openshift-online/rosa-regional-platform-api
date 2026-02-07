permit(
  ?principal,
  action == ROSA::Action::"UpdateAccessEntry",
  resource
)
when { context.policyArn == "arn:aws:rosa::aws:cluster-access-policy/ROSAViewPolicy" || context.policyArn == "arn:aws:rosa::aws:cluster-access-policy/ROSAEditPolicy" };

forbid(
  ?principal,
  action == ROSA::Action::"UpdateAccessEntry",
  resource
)
when { context.policyArn == "arn:aws:rosa::aws:cluster-access-policy/ROSAClusterAdminPolicy" || context.policyArn == "arn:aws:rosa::aws:cluster-access-policy/ROSAAdminPolicy" };

permit(
  ?principal,
  action in [ROSA::Action::"CreateAccessEntry", ROSA::Action::"DeleteAccessEntry", ROSA::Action::"DescribeAccessEntry", ROSA::Action::"ListAccessEntries", ROSA::Action::"ListAccessPolicies"],
  resource
);
