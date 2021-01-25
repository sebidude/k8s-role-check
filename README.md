# k8s-role-check

Check bindings, subjects and pods which make use of certain Role or ClusterRole

## build

```
go build -o k8s-role-check ./cmd/k8s-role-check/main.go
```

## run

```
# check a Role
./k8s-role-check role name-of-the-role

# check a ClusterRole
./k8s-role-check clusterrole name-of-the-cluster-role

```

