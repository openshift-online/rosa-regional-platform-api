permit(
  ?principal,
  action == ROSA::Action::"CreateAccessEntry",
  resource
)
when { context.principalArn like "arn:aws:iam::111122223333:role/developers-*" || context.principalArn like "arn:aws:iam::111122223333:role/operators-*" };

forbid(
  ?principal,
  action == ROSA::Action::"CreateAccessEntry",
  resource
)
when { context.principalArn like "arn:aws:iam::111122223333:role/admin*" || context.principalArn like "arn:aws:iam::111122223333:role/Administrator*" };
