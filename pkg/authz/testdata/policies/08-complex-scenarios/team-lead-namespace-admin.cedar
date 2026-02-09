permit(
  ?principal,
  action in [ROSA::Action::"DescribeCluster", ROSA::Action::"DescribeAccessEntry", ROSA::Action::"ListClusters", ROSA::Action::"ListAccessEntries", ROSA::Action::"ListAccessPolicies"],
  resource
);

permit(
  ?principal,
  action in [ROSA::Action::"CreateAccessEntry", ROSA::Action::"DeleteAccessEntry", ROSA::Action::"UpdateAccessEntry"],
  resource
)
when { resource.tags["Team"] == context.principalTags["Team"] };

forbid(
  ?principal,
  action == ROSA::Action::"UpdateAccessEntry",
  resource
)
when { context.accessScope == "namespace" }
unless { context.allowedNamespaces.containsAll(context.namespaces) };

forbid(
  ?principal,
  action == ROSA::Action::"UpdateAccessEntry",
  resource
)
when { context.accessScope == "cluster" };

forbid(
  ?principal,
  action == ROSA::Action::"UpdateAccessEntry",
  resource
)
when { context.policyArn like "*ROSAClusterAdminPolicy" || context.policyArn like "*ROSAAdminPolicy" };

forbid(
  ?principal,
  action in [ROSA::Action::"DeleteAccessEntry", ROSA::Action::"UpdateAccessEntry"],
  resource
)
unless { resource.tags["Team"] == context.principalTags["Team"] };
