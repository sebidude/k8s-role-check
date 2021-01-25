package main

import (
	"context"
	"fmt"
	"os"
	"os/user"
	"path/filepath"

	corev1 "k8s.io/api/core/v1"
	rbac "k8s.io/api/rbac/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	kingpin "gopkg.in/alecthomas/kingpin.v2"
)

var (
	gitcommit               string
	appversion              string
	buildtime               string
	clientconfig            *rest.Config
	clientset               *kubernetes.Clientset
	namespace               string
	kubeconfig              string
	resource                string
	rolename                string
	serviceAccountsInUse    []rbac.Subject
	groupsInUse             []rbac.Subject
	usersInUse              []rbac.Subject
	podsInUse               []corev1.Pod
	roleBindingsInUse       []rbac.RoleBinding
	clusterRoleBindingInUse []rbac.ClusterRoleBinding
)

type kclient struct {
	clientset *kubernetes.Clientset
}

func main() {

	app := kingpin.New("k8s-role-check", "check usage of roles or clusterroles in k8s")
	app.Flag("kubeconfig", "Path to the kubectl config.").Short('c').Envar("KUBECONFIG").StringVar(&kubeconfig)
	app.Arg("resource", "role or clusterrole to check").StringVar(&resource)
	app.Arg("rolename", "name of the resource").StringVar(&rolename)

	kingpin.MustParse(app.Parse(os.Args[1:]))

	if len(kubeconfig) < 1 {
		// we try the find the config at the default path.
		// https://kubernetes.io/docs/concepts/configuration/organize-cluster-access-kubeconfig/

		currentUser, _ := user.Current()
		if currentUser != nil {
			if len(currentUser.HomeDir) > 0 {
				kubeConfigPath := filepath.Join(currentUser.HomeDir, ".kube", "config")
				_, err := os.Stat(kubeConfigPath)
				if os.IsNotExist(err) && err != nil {
					kubeconfig = ""
				} else {
					kubeconfig = kubeConfigPath
				}
			}
		}
	}

	if len(kubeconfig) < 1 {
		config, err := rest.InClusterConfig()
		if err != nil {
			panic(err.Error())
		}
		clientconfig = config
	} else {
		config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
		if err != nil {
			panic(err.Error())
		}
		clientconfig = config
	}
	var err error
	clientset, err = kubernetes.NewForConfig(clientconfig)
	if err != nil {
		panic(err.Error())
	}

	if namespace == "" {
		var err error
		namespace, _, err = clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
			clientcmd.NewDefaultClientConfigLoadingRules(),
			&clientcmd.ConfigOverrides{},
		).Namespace()
		if err != nil {
			panic(err.Error())
		}
	}

	client := new(kclient)
	client.clientset = clientset

	switch resource {
	case "role":
		fmt.Printf("RoleBindings for %s:\n", rolename)
		bindings, err := clientset.RbacV1().RoleBindings("").List(context.Background(), v1.ListOptions{})
		if err != nil {
			panic(err.Error())
		}
		for _, binding := range bindings.Items {
			if binding.RoleRef.Name == rolename {
				fmt.Printf("  \u251c\u2500 %-25s - %s\n", binding.Namespace, binding.Name)
				for _, sub := range binding.Subjects {
					switch sub.Kind {
					case "ServiceAccount":
						serviceAccountsInUse = append(serviceAccountsInUse, sub)
					case "User":
						usersInUse = append(usersInUse, sub)
					case "Group":
						groupsInUse = append(groupsInUse, sub)
					}
				}
			}
		}

	case "clusterrole":
		bindings, err := clientset.RbacV1().ClusterRoleBindings().List(context.Background(), v1.ListOptions{})
		if err != nil {
			panic(err.Error())
		}
		fmt.Printf("ClusterRoleBindings for %s:\n", rolename)
		for _, binding := range bindings.Items {
			if binding.RoleRef.Name == rolename {
				fmt.Printf("  \u251c\u2500 %-25s - %s\n", binding.Namespace, binding.Name)
				for _, sub := range binding.Subjects {
					switch sub.Kind {
					case "ServiceAccount":
						serviceAccountsInUse = append(serviceAccountsInUse, sub)
					case "User":
						usersInUse = append(usersInUse, sub)
					case "Group":
						groupsInUse = append(groupsInUse, sub)
					}
				}
			}
		}

	}
	fmt.Println("")
	printList("ServiceAccounts", serviceAccountsInUse)
	printList("Users", usersInUse)
	printList("Groups", groupsInUse)

	fmt.Println("")
	fmt.Printf("Pods which use the SA from above:\n")
	for _, sa := range serviceAccountsInUse {
		err := client.getPodsWithServiceAccount(sa)
		if err != nil {
			fmt.Printf("Failed to get pods with service account '%s'", sa.Name)
		}
		if len(podsInUse) == 0 {
			continue
		}
		fmt.Printf("ServiceAccount: %s - Namespace: %s\n", sa.Name, sa.Namespace)
		for i := 0; i < len(podsInUse); i++ {
			if i < len(podsInUse)-1 {
				fmt.Printf("  \u251c\u2500 %s\n", podsInUse[i].Name)
			} else {
				fmt.Printf("  \u2514\u2500 %s\n\n", podsInUse[i].Name)
			}
		}
		podsInUse = make([]corev1.Pod, 0)

	}
}

func (c *kclient) getPodsWithServiceAccount(sub rbac.Subject) error {
	pods, err := c.clientset.CoreV1().Pods(sub.Namespace).List(context.Background(), v1.ListOptions{})
	if err != nil {
		return err
	}
	for _, pod := range pods.Items {
		if pod.Spec.ServiceAccountName == sub.Name {
			podsInUse = append(podsInUse, pod)
		}
	}
	return nil
}

func printList(kind string, subs []rbac.Subject) {
	if len(subs) > 0 {
		fmt.Printf("%s bound to %s: %s\n", kind, resource, rolename)
		for i := 0; i < len(subs); i++ {
			if i < len(subs)-1 {
				fmt.Printf("  \u251c\u2500 %-25s (%s)\n", subs[i].Name, subs[i].Namespace)
			} else {
				fmt.Printf("  \u2514\u2500 %-25s (%s)\n\n", subs[i].Name, subs[i].Namespace)
			}
		}
	}
}
