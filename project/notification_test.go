package project

import (
	"testing"

	"gopkg.in/yaml.v3"
)

// TestNotificationComponentAndResourceRemainOptIn preserves existing default projects and App ownership.
func TestNotificationComponentAndResourceRemainOptIn(t *testing.T) {
	definition, ok := ComponentDefinitionByKey(ComponentNotification)
	if !ok || definition.DefaultSelected {
		t.Fatal("notification component is not opt-in")
	}
	resource, ok := ResourceDefinitionByKey(ResourceNotification)
	if !ok || resource.AppliesTo(Components{}) {
		t.Fatal("disabled resource is applicable")
	}
	var components Components
	if err := yaml.Unmarshal([]byte("[notification]"), &components); err != nil {
		t.Fatal(err)
	}
	if !components.Notification || !resource.AppliesTo(components) {
		t.Fatal("component did not enable resource")
	}
	selected, err := AppComponentsFromKeys(Components{}, []ComponentKey{ComponentNotification})
	if err != nil || !selected.Notification {
		t.Fatalf("App component = %#v, %v", selected, err)
	}
	if !PromoteAppComponents(Components{}, selected).Notification {
		t.Fatal("named App resource was not included in project envelope")
	}
	selection, err := resource.DefaultSelection(components)
	if err != nil || selection.Active != "log" || len(selection.Supported) != 1 {
		t.Fatalf("defaults = %#v, %v", selection, err)
	}
	driver, ok := resource.Driver("webhook")
	if !ok || driver.Service != ServiceNotificationWebhook || driver.LocallyProvisionable {
		t.Fatal("webhook endpoint must remain externally configured")
	}
}
