forbid(
  ?principal,
  action,
  resource
)
when { context.mfaPresent == "false" };

forbid(
  ?principal,
  action in [ROSA::Action::"DeleteCluster", ROSA::Action::"DeleteNodePool", ROSA::Action::"DeleteAccessEntry"],
  resource
)
unless { context.sourceIp like "10.*" || context.sourceIp like "192.168.1.*" };

permit(
  ?principal,
  action,
  resource
);
