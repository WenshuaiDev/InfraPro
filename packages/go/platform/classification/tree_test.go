package classification

import (
	"fmt"
	"github.com/google/uuid"
	"testing"
)

func TestCategoryTreeIdentityDepthAndNoImplicitLeafMigration(t *testing.T) {
	root := uuid.NewString()
	c := Content{DictionaryCode: DictionaryCode, SensorRootID: root, Nodes: []Node{{ID: root, Code: "root", Name: "名称可以不含传感器", Status: "active", Selectable: true}}}
	parent := root
	for i := 1; i < 8; i++ {
		id := uuid.NewString()
		c.Nodes = append(c.Nodes, Node{ID: id, Code: fmt.Sprintf("n%d", i), Name: "同名", ParentID: parent, Status: "active", Selectable: true})
		parent = id
	}
	v, e := Normalize(c, true)
	if e != nil {
		t.Fatal(e)
	}
	for _, n := range v.Nodes {
		if !n.Sensor {
			t.Fatal("sensor identity ignored")
		}
		if n.ID == root && !n.Selectable {
			t.Fatal("parent direct classification disabled")
		}
	}
	bad := c
	bad.Nodes = append([]Node{}, c.Nodes...)
	bad.Nodes[0].ParentID = parent
	if _, e = Normalize(bad, true); e == nil {
		t.Fatal("cycle accepted")
	}
	bad.Nodes[0].ParentID = uuid.NewString()
	if _, e = Normalize(bad, true); e == nil {
		t.Fatal("foreign parent accepted")
	}
	bad = c
	bad.DictionaryCode = "other"
	if _, e = Normalize(bad, true); e == nil {
		t.Fatal("cross dictionary accepted")
	}
	bad = c
	bad.SensorRootID = ""
	if _, e = Normalize(bad, true); e == nil {
		t.Fatal("sensor meaning inferred from names")
	}
	for i := 8; i < 129; i++ {
		id := uuid.NewString()
		c.Nodes = append(c.Nodes, Node{ID: id, Code: fmt.Sprintf("n%d", i), Name: "节点", ParentID: parent, Status: "active", Selectable: true})
		parent = id
	}
	if _, e = Normalize(c, true); e == nil {
		t.Fatal("technical depth silently truncated")
	}
}
