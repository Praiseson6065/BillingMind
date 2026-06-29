package ontology

import (
	"encoding/json"
	"os"
)

type Ontology struct {
	Context map[string]string `json:"@context"`
	Graph   []Entity          `json:"@graph"`
}

type Entity struct {
	ID           string            `json:"@id"`
	Type         string            `json:"@type"`
	Properties   []string          `json:"properties,omitempty"`
	Relations    map[string]string `json:"relations,omitempty"`
	StatusValues []string          `json:"statusValues,omitempty"`
	TaskTypes    []string          `json:"taskTypes,omitempty"`
}

func LoadOntology(path string) (*Ontology, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var ont Ontology
	if err := json.Unmarshal(data, &ont); err != nil {
		return nil, err
	}
	return &ont, nil
}
func (o *Ontology) GetEntity(id string) *Entity {
	for i := range o.Graph {
		if o.Graph[i].ID == id {
			return &o.Graph[i]
		}
	}
	return nil
}
