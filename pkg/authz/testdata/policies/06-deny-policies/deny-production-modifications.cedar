forbid(
  ?principal,
  action in [ROSA::Action::"DeleteCluster", ROSA::Action::"DeleteNodePool", ROSA::Action::"UpdateClusterConfig", ROSA::Action::"UpdateClusterVersion", ROSA::Action::"UpdateNodePool", ROSA::Action::"ScaleNodePool"],
  resource
)
when { resource.tags["Environment"] == "production" };
