forbid(
  ?principal,
  action in [ROSA::Action::"TagResource", ROSA::Action::"UntagResource"],
  resource
)
when { context.tagKeys.contains("Environment") || context.tagKeys.contains("Owner") || context.tagKeys.contains("CostCenter") };

forbid(
  ?principal,
  action == ROSA::Action::"CreateCluster",
  resource
)
unless { context has requestTags && context.requestTags has "Environment" };
