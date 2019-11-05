package gorm

import (
	"errors"
	"fmt"
	"github.com/magezeng/tipujson"
	"reflect"
)

type TipuSqlScanner struct {
	Type  reflect.Type
	Value reflect.Value
}

func (scanner *TipuSqlScanner) Scan(src interface{}) error {
	if srcString, ok := src.(string); ok {
		return tipujson.StringToObjByReflect(srcString, scanner.Type, scanner.Value)
	}
	tempPrint := fmt.Sprintf("%v", scanner.Type) + "   " + fmt.Sprintf("%v", scanner.Value)
	fmt.Println(tempPrint)
	if srcUint8Slice, ok := src.([]uint8); ok {
		return tipujson.StringToObjByReflect(string(srcUint8Slice), scanner.Type, scanner.Value)
	}
	return errors.New("TipuSqlScanner 的src 必须为字符串")
}
