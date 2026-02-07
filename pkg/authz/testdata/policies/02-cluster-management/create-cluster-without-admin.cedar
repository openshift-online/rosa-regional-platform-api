permit(
  ?principal,
  action == ROSA::Action::"CreateCluster",
  resource
)
when { context.bootstrapClusterCreatorAdminPermissions == "false" };
