package gorm

import (
	"database/sql"
	"go/ast"
	"reflect"
	"strings"
	"time"

	"github.com/jinzhu/inflection"
)

// TableName returns model's table name
func (s *ModelStruct) TableName(db *DB) string {
	s.l.Lock()
	defer s.l.Unlock()
	if s.defaultTableName == "" && db != nil && s.ModelType != nil {
		// Set default table name
		if tabler, ok := reflect.New(s.ModelType).Interface().(tabler); ok {
			s.defaultTableName = tabler.TableName()
		} else {
			tableName := ToTableName(s.ModelType.Name())
			db.parent.RLock()
			if db.parent != nil && !db.parent.singularTable {
				tableName = inflection.Plural(tableName)
			}
			db.parent.RUnlock()
			s.defaultTableName = tableName
		}
	}
	return DefaultTableNameHandler(db, s.defaultTableName)
}

// TagSettingsSet Sets a tag in the tag settings map
func (sf *StructField) TagSettingsSet(key, val string) {
	sf.tagSettingsLock.Lock()
	defer sf.tagSettingsLock.Unlock()
	sf.TagSettings[key] = val
}

// TagSettingsGet returns a tag from the tag settings
func (sf *StructField) TagSettingsGet(key string) (string, bool) {
	sf.tagSettingsLock.RLock()
	defer sf.tagSettingsLock.RUnlock()
	val, ok := sf.TagSettings[key]
	return val, ok
}

// TagSettingsDelete deletes a tag
func (sf *StructField) TagSettingsDelete(key string) {
	sf.tagSettingsLock.Lock()
	defer sf.tagSettingsLock.Unlock()
	delete(sf.TagSettings, key)
}

func getForeignField(column string, fields []*StructField) *StructField {
	for _, field := range fields {
		if field.Name == column || field.DBName == column || field.DBName == ToColumnName(column) {
			return field
		}
	}
	return nil
}

// GetModelStruct get value's model struct, relationships based on struct and tag definition
func (scope *Scope) GetModelStruct() *ModelStruct {
	var modelStruct ModelStruct
	// Scope value can't be nil
	if scope.Value == nil {
		return &modelStruct
	}

	reflectType := reflect.ValueOf(scope.Value).Type()
	for reflectType.Kind() == reflect.Slice || reflectType.Kind() == reflect.Ptr {
		reflectType = reflectType.Elem()
	}

	// Scope value need to be a struct
	if reflectType.Kind() != reflect.Struct {
		return &modelStruct
	}

	// Get Cached model struct
	isSingularTable := false
	if scope.db != nil && scope.db.parent != nil {
		scope.db.parent.RLock()
		isSingularTable = scope.db.parent.singularTable
		scope.db.parent.RUnlock()
	}

	hashKey := struct {
		singularTable bool
		reflectType   reflect.Type
	}{isSingularTable, reflectType}

	if value, ok := modelStructsMap.Load(hashKey); ok && value != nil {
		return value.(*ModelStruct)
	}

	modelStruct.ModelType = reflectType
	// Get all fields
	for i := 0; i < reflectType.NumField(); i++ {
		if fieldStruct := reflectType.Field(i); ast.IsExported(fieldStruct.Name) {
			// Update field Info
			field := &StructField{
				Struct:      fieldStruct,
				Name:        fieldStruct.Name,
				Names:       []string{fieldStruct.Name},
				Tag:         fieldStruct.Tag,
				TagSettings: parseTagSetting(fieldStruct.Tag),
			}

			// is ignored field
			// field.IsNormal将决定定义的字段类型能否被mysql识别从而创建
			if _, ok := field.TagSettingsGet("-"); ok {
				field.IsIgnored = true
			} else {
				// 判定是否是主键
				if _, ok := field.TagSettingsGet("PRIMARY_KEY"); ok {
					field.IsPrimaryKey = true
					modelStruct.PrimaryFields = append(modelStruct.PrimaryFields, field)
				}
				// 是否有默认值
				if _, ok := field.TagSettingsGet("DEFAULT"); ok && !field.IsPrimaryKey {
					field.HasDefaultValue = true
				}

				// 是否自增
				if _, ok := field.TagSettingsGet("AUTO_INCREMENT"); ok && !field.IsPrimaryKey {
					field.HasDefaultValue = true
				}

				// 利用indirectType.Kind()结果判断字段类型，进行相关操作
				indirectType := fieldStruct.Type

				for indirectType.Kind() == reflect.Ptr {
					indirectType = indirectType.Elem()
				}

				fieldValue := reflect.New(indirectType).Interface()
				if _, isScanner := fieldValue.(sql.Scanner); isScanner {
					// is scanner
					field.IsScanner, field.IsNormal = true, true
					if indirectType.Kind() == reflect.Struct {
						for i := 0; i < indirectType.NumField(); i++ {
							for key, value := range parseTagSetting(indirectType.Field(i).Tag) {
								if _, ok := field.TagSettingsGet(key); !ok {
									field.TagSettingsSet(key, value)
								}
							}
						}
					}
				} else if _, isTime := fieldValue.(*time.Time); isTime {
					// is time
					field.IsNormal = true
				} else if _, ok := field.TagSettingsGet("EMBEDDED"); ok || fieldStruct.Anonymous {
					// is embedded struct
					for _, subField := range scope.New(fieldValue).GetModelStruct().StructFields {
						subField = subField.clone()
						subField.Names = append([]string{fieldStruct.Name}, subField.Names...)
						if prefix, ok := field.TagSettingsGet("EMBEDDED_PREFIX"); ok {
							subField.DBName = prefix + subField.DBName
						}

						if subField.IsPrimaryKey {
							if _, ok := subField.TagSettingsGet("PRIMARY_KEY"); ok {
								modelStruct.PrimaryFields = append(modelStruct.PrimaryFields, subField)
							} else {
								subField.IsPrimaryKey = false
							}
						}
						if subField.Relationship != nil && subField.Relationship.JoinTableHandler != nil {
							if joinTableHandler, ok := subField.Relationship.JoinTableHandler.(*JoinTableHandler); ok {
								newJoinTableHandler := &JoinTableHandler{}
								newJoinTableHandler.Setup(subField.Relationship, joinTableHandler.TableName, reflectType, joinTableHandler.Destination.ModelType)
								subField.Relationship.JoinTableHandler = newJoinTableHandler
							}
						}
						modelStruct.StructFields = append(modelStruct.StructFields, subField)
					}
					continue
				} else {
					field.IsNormal = true
				}
			}

			// 即使被忽略，也可以将db值解码到字段中
			if value, ok := field.TagSettingsGet("COLUMN"); ok {
				field.DBName = value
			} else {
				field.DBName = ToColumnName(fieldStruct.Name)
			}
			modelStruct.StructFields = append(modelStruct.StructFields, field)
		}
	}

	if len(modelStruct.PrimaryFields) == 0 {
		if field := getForeignField("id", modelStruct.StructFields); field != nil {
			field.IsPrimaryKey = true
			modelStruct.PrimaryFields = append(modelStruct.PrimaryFields, field)
		}
	}
	modelStructsMap.Store(hashKey, &modelStruct)
	return &modelStruct
}

// GetStructFields get model's field structs
func (scope *Scope) GetStructFields() (fields []*StructField) {
	return scope.GetModelStruct().StructFields
}

func parseTagSetting(tags reflect.StructTag) map[string]string {
	setting := map[string]string{}
	for _, str := range []string{tags.Get("sql"), tags.Get("gorm")} {
		if str == "" {
			continue
		}
		tags := strings.Split(str, ";")
		for _, value := range tags {
			v := strings.Split(value, ":")
			k := strings.TrimSpace(strings.ToUpper(v[0]))
			if len(v) >= 2 {
				setting[k] = strings.Join(v[1:], ":")
			} else {
				setting[k] = k
			}
		}
	}
	return setting
}
