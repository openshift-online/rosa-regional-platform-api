forbid(
  ?principal,
  action == ROSA::Action::"CreateAccessEntry",
  resource
)
when { context.kubernetesGroups.contains("system:masters") };

permit(
  ?principal,
  action == ROSA::Action::"CreateAccessEntry",
  resource
);
