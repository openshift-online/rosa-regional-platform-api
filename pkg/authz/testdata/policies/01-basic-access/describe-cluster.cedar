permit(
  ?principal,
  action == ROSA::Action::"DescribeCluster",
  resource == ROSA::Resource::"arn:aws:rosa:us-west-2:111122223333:cluster/my-cluster"
);
