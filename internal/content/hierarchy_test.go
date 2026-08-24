package content

import "testing"

func TestHierarchyAncestorsDescendantsAndOrder(t *testing.T) {
	tree, err := NewHierarchy([]HierarchyNode{
		{EntryID: "company", Slug: "company", Title: "Company"},
		{EntryID: "services", Slug: "services", ParentEntryID: "company", Title: "Services", MenuOrder: 2},
		{EntryID: "about", Slug: "about", ParentEntryID: "company", Title: "About", MenuOrder: 1},
		{EntryID: "team", Slug: "team", ParentEntryID: "about", Title: "Team"},
	})
	if err != nil {
		t.Fatal(err)
	}
	children := tree.Children("company")
	if len(children) != 2 || children[0].EntryID != "about" || children[1].EntryID != "services" {
		t.Fatalf("unexpected sibling order: %#v", children)
	}
	if ancestors := tree.Ancestors("team"); len(ancestors) != 2 || ancestors[0].EntryID != "about" || ancestors[1].EntryID != "company" {
		t.Fatalf("unexpected ancestors: %#v", ancestors)
	}
	if descendants := tree.Descendants("company"); len(descendants) != 3 {
		t.Fatalf("unexpected descendants: %#v", descendants)
	}
}

func TestHierarchyRejectsCycle(t *testing.T) {
	_, err := NewHierarchy([]HierarchyNode{{EntryID: "a", ParentEntryID: "b"}, {EntryID: "b", ParentEntryID: "a"}})
	if err == nil {
		t.Fatal("expected cycle rejection")
	}
}
