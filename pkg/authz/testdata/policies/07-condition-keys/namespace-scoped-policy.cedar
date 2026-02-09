permit(
  ?principal,
  action == ROSA::Action::"UpdateAccessEntry",
  resource
)
when { context.accessScope == "namespace" };

forbid(
  ?principal,
  action == ROSA::Action::"UpdateAccessEntry",
  resource
)
when { context.accessScope == "cluster" };
