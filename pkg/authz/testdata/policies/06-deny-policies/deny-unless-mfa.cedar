forbid(
  ?principal,
  action in [ROSA::Action::"DeleteCluster", ROSA::Action::"DeleteNodePool", ROSA::Action::"DeleteAccessEntry"],
  resource
)
when { context.mfaPresent == "false" };
