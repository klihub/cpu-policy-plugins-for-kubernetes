// Copyright 2019 Intel Corporation. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package sysfs

import (
	"fmt"
	"strings"
	"strconv"
	"os"
	"path/filepath"
	"io/ioutil"
)


// Get the trailing enumeration part of a name.
func getEnumeratedId(name string) Id {
	id := 0
	base := 1
	for idx := len(name) - 1; idx > 0; idx-- {
		d := name[idx]

		if '0' <= d && d <= '9' {
			id += base * (int(d) - '0')
			base *= 10
		} else {
			if base > 1 {
				return Id(id)
			}

			return Id(-1)
		}
	}

	return Id(-1)
}

// Read content of a sysfs entry and convert it according to the type of a given pointer.
func readSysfsEntry(base, entry string, ptr interface{}, args ...interface{}) (string, error) {
	var buf string

	path := filepath.Join(base, entry)

	if blob, err := ioutil.ReadFile(path); err != nil {
		return "", sysfsError(path, "failed to read sysfs entry: %v", err)
	} else {
		buf = strings.Trim(string(blob), "\n")
	}

	if ptr == interface{}(nil) {
		return buf, nil
	}

	switch ptr.(type) {
	case *string, *Id, *int, *uint, *int8, *uint8, *int16, *uint16, *int32, *uint32, *int64, *uint64:
		err := parseValue(buf, ptr)
		if err != nil {
			return "", sysfsError(path, "%v", err)
		}
		return buf, nil

	case *IdSet, *[]int, *[]uint, *[]int8, *[]uint8, *[]int16, *[]uint16, *[]int32, *[]uint32, *[]int64, *[]uint64:
		sep, err := getSeparator(" ", args)
		if err != nil {
			return "", sysfsError(path, "%v", err)
		}
		err = parseValueList(buf, sep, ptr)
		if err != nil {
			return "", sysfsError(path, "%v", err)
		}
		return buf, nil
	}

	return "", sysfsError(path, "unsupported sysfs entry type %T", ptr)
}

// Write a value to a sysfs entry. An optional item separator can be specified for slice values.
func writeSysfsEntry(base, entry string, val, oldp interface{}, args ...interface{}) (string, error) {
	var buf, old string
	var err error

	if oldp != nil {
		if old, err = readSysfsEntry(base, entry, oldp, args...); err != nil {
			return "", err
		}
	}

	path := filepath.Join(base, entry)

	switch val.(type) {
	case string, Id, int, uint, int8, uint8, int16, uint16, int32, uint32, int64, uint64:
		buf, err = formatValue(val)
		if err != nil {
			return "", sysfsError(path, "%v", err)
		}

	case IdSet, []int, []uint, []int8, []uint8, []int16, []uint16, []int32, []uint32, []int64, []uint64:
		sep, err := getSeparator(" ", args)
		if err != nil {
			return "", sysfsError(path, "%v",err)
		}
		buf, err = formatValueList(sep, val)
		if err != nil {
			return "", sysfsError(path, "%v", err)
		}

	default:
		return "", sysfsError(path, "unsupported sysfs entry type %T", val)
	}

	f, err := os.OpenFile(path, os.O_WRONLY, 0)
	if err != nil {
		return "", sysfsError(path, "cannot open: %v", err)
	}
	defer f.Close()

	if _, err = f.Write([]byte(buf + "\n")); err != nil {
		return "", sysfsError(path, "cannot write: %v", err)
	}

	return old, nil
}

// Determine list separator string, given an optional separator variadic argument.
func getSeparator(defaultVal string, args []interface{}) (string, error) {
	switch len(args) {
	case 0: return defaultVal, nil
	case 1: return args[0].(string), nil
	}

	return "", fmt.Errorf("invalid separator (%v), 1 expected, %d given", args, len(args))
}

// Parse a value from a string.
func parseValue(str string, value interface{}) error {
	switch value.(type) {
	case *string:
		*value.(*string) = str

	case *Id, *int, *int8, *int16, *int32, *int64:
		v, err := strconv.ParseInt(str, 0, 0)
		if err != nil {
			return fmt.Errorf("invalid entry '%s': %v", str, err)
		}

		switch value.(type) {
		case *Id:   *value.(*Id) = Id(v)
		case *int:   *value.(*int) = int(v)
		case *int8:  *value.(*int8) = int8(v)
		case *int16: *value.(*int16) = int16(v)
		case *int32: *value.(*int32) = int32(v)
			case int64:  *value.(*int64) = v
		}

	case *uint, *uint8, *uint16, *uint32, *uint64:
		v, err := strconv.ParseUint(str, 0, 0)
		if err != nil {
			return fmt.Errorf("invalid entry: '%s': %v", str, err)
		}

		switch value.(type) {
		case *uint:   *value.(*uint) = uint(v)
		case *uint8:  *value.(*uint8) = uint8(v)
		case *uint16: *value.(*uint16) = uint16(v)
		case *uint32: *value.(*uint32) = uint32(v)
		case *uint64: *value.(*uint64) = v
		}
	}

	return nil
}

// Parse a list of values from a string into a slice.
func parseValueList(str, sep string, valuep interface{}) error {
	var value interface{}

	switch valuep.(type) {
	case *IdSet:    value = NewIdSet()
	case *[]int:    value = []int{}
	case *[]uint:   value = []uint{}
	case *[]int8:   value = []int8{}
	case *[]uint8:  value = []uint8{}
	case *[]int16:  value = []int16{}
	case *[]uint16: value = []uint16{}
	case *[]int32:  value = []int32{}
	case *[]uint32: value = []uint32{}
	case *[]int64:  value = []int64{}
	case *[]uint64: value = []uint64{}
	default:
		return fmt.Errorf("invalid slice value type: %T", valuep)
	}

	for _, s := range strings.Split(str, sep) {
		switch value.(type) {
		case IdSet:
			if rng := strings.Split(s, "-"); len(rng) == 1 {
				id, err := strconv.Atoi(s)
				if err != nil {
					return fmt.Errorf("invalid entry '%s': %v", s, err)
				}
				value.(IdSet).Add(Id(id))
			} else {
				beg, err := strconv.Atoi(rng[0])
				if err != nil {
					return fmt.Errorf("invalid entry '%s': %v", s, err)
				}
				end, err := strconv.Atoi(rng[1])
				if err != nil {
					return fmt.Errorf("invalid entry '%s': %v", s, err)
				}
				for id := beg; id <= end; id++ {
					value.(IdSet).Add(Id(id))
				}
			}

		case []int, []int8, []int16, []int32, []int64:
			v, err := strconv.ParseInt(s, 0, 0)
			if err != nil {
				return fmt.Errorf("invalid entry '%s': %v", s, err)
			}
			switch value.(type) {
			case []int:   value = append(value.([]int), int(v))
			case []int8:  value = append(value.([]int8), int8(v))
			case []int16: value = append(value.([]int16), int16(v))
			case []int32: value = append(value.([]int32), int32(v))
			case []int64: value = append(value.([]int64), v)
			}

		case []uint, []uint8, []uint16, []uint32, []uint64:
			v, err := strconv.ParseUint(s, 0, 0)
			if err != nil {
				return fmt.Errorf("invalid entry '%s': %v", s, err)
			}
			switch value.(type) {
			case []uint:   value = append(value.([]uint), uint(v))
			case []uint8:  value = append(value.([]uint8), uint8(v))
			case []uint16: value = append(value.([]uint16), uint16(v))
			case []uint32: value = append(value.([]uint32), uint32(v))
			case []uint64: value = append(value.([]uint64), v)
			}
		}
	}

	switch valuep.(type) {
	case *IdSet:    *valuep.(*IdSet) = value.(IdSet)
	case *[]int:    *valuep.(*[]int) = value.([]int)
	case *[]uint:   *valuep.(*[]uint) = value.([]uint)
	case *[]int8:   *valuep.(*[]int8) = value.([]int8)
	case *[]uint8:  *valuep.(*[]uint8) = value.([]uint8)
	case *[]int16:  *valuep.(*[]int16) = value.([]int16)
	case *[]uint16: *valuep.(*[]uint16) = value.([]uint16)
	case *[]int32:  *valuep.(*[]int32) = value.([]int32)
	case *[]uint32: *valuep.(*[]uint32) = value.([]uint32)
	case *[]int64:  *valuep.(*[]int64) = value.([]int64)
	case *[]uint64: *valuep.(*[]uint64) = value.([]uint64)
	}

	return nil
}

// Format a value into a string.
func formatValue(value interface{}) (string, error) {
	switch value.(type) {
	case string:
		return value.(string), nil
	case Id, int, uint, int8, uint8, int16, uint16, int32, uint32, int64, uint64:
		return fmt.Sprintf("%d", value), nil
	default:
		return "", fmt.Errorf("invalid value type %T", value)
	}
}

// Format a list of values from a slice into a string.
func formatValueList(sep string, value interface{}) (string, error) {
	var v []interface{}

	switch value.(type) {
	case IdSet:
		return value.(IdSet).StringWithSeparator(sep), nil
	case []int, []uint, []int8, []uint8, []int16, []uint16, []int32, []uint32, []int64, []uint64:
		v = value.([]interface{})
	default:
		return "", fmt.Errorf("invalid value type %T", value)
	}

	str := ""
	t := ""
	for idx, _ := range v {
		str = str + t + fmt.Sprintf("%d", v[idx])
		t = sep
	}

	return "", nil
}
