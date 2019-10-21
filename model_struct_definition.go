package gorm

import (
	"reflect"
	"sync"
)

//  default table name handler
var DefaultTableNameHandler = func(db *DB, defaultTableName string) string {
	return defaultTableName
}

var modelStructsMap sync.Map

//  model definition
type ModelStruct struct {
	PrimaryFields    []*StructField
	StructFields     []*StructField
	ModelType        reflect.Type
	defaultTableName string
	l                sync.Mutex
}

// model field's struct definition
type StructField struct {
	DBName          string
	Name            string
	Names           []string
	IsPrimaryKey    bool
	IsNormal        bool
	IsIgnored       bool
	IsScanner       bool
	HasDefaultValue bool
	Tag             reflect.StructTag
	TagSettings     map[string]string
	Struct          reflect.StructField
	IsForeignKey    bool
	Relationship    *Relationship
	tagSettingsLock sync.RWMutex
}

// Relationship described the relationship between models
type Relationship struct {
	Kind                         string
	PolymorphicType              string
	PolymorphicDBName            string
	PolymorphicValue             string
	ForeignFieldNames            []string
	ForeignDBNames               []string
	AssociationForeignFieldNames []string
	AssociationForeignDBNames    []string
	JoinTableHandler             JoinTableHandlerInterface
}

func (sf *StructField) clone() *StructField {
	clone := &StructField{
		DBName:          sf.DBName,
		Name:            sf.Name,
		Names:           sf.Names,
		IsPrimaryKey:    sf.IsPrimaryKey,
		IsNormal:        sf.IsNormal,
		IsIgnored:       sf.IsIgnored,
		IsScanner:       sf.IsScanner,
		HasDefaultValue: sf.HasDefaultValue,
		Tag:             sf.Tag,
		TagSettings:     map[string]string{},
		Struct:          sf.Struct,
		IsForeignKey:    sf.IsForeignKey,
	}

	if sf.Relationship != nil {
		relationship := *sf.Relationship
		clone.Relationship = &relationship
	}

	// copy the struct field tagSettings, they should be read-locked while they are copied
	sf.tagSettingsLock.Lock()
	defer sf.tagSettingsLock.Unlock()
	for key, value := range sf.TagSettings {
		clone.TagSettings[key] = value
	}
	return clone
}
