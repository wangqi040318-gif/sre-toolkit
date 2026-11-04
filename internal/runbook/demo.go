package runbook

// DemoSnapshot is a failed checkout release. Revision 12 shipped
// ghcr.io/example/checkout:1.9.0 and no pod is Ready. Revision 11 is the
// last good image, ghcr.io/example/checkout:1.8.4.
func DemoSnapshot() Snapshot {
	current := []ContainerImage{{Name: "checkout", Image: "ghcr.io/example/checkout:1.9.0"}}
	previous := []ContainerImage{{Name: "checkout", Image: "ghcr.io/example/checkout:1.8.4"}}
	return Snapshot{
		Cluster:         "demo-cluster",
		Kind:            KindDeployment,
		Namespace:       "shop",
		Name:            "checkout",
		Selector:        "app=checkout",
		CurrentRevision: 12,
		Desired:         3,
		Ready:           0,
		Updated:         3,
		Images:          current,
		Revisions: []Revision{
			{Number: 11, Images: previous},
			{Number: 12, Images: current},
		},
		Failure: "3 pods are CrashLoopBackOff; container checkout exits because STRIPE_API_KEY is missing in the 1.9.0 image.",
	}
}
