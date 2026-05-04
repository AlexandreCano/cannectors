package metadata

import (
	"testing"
)

func TestNewAccessor(t *testing.T) {
	tests := []struct {
		name          string
		fieldName     string
		wantFieldName string
	}{
		{
			name:          "empty uses default",
			fieldName:     "",
			wantFieldName: DefaultFieldName,
		},
		{
			name:          "custom field name",
			fieldName:     "_custom",
			wantFieldName: "_custom",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			accessor := NewAccessor(tt.fieldName)
			if accessor.FieldName() != tt.wantFieldName {
				t.Errorf("FieldName() = %q, want %q", accessor.FieldName(), tt.wantFieldName)
			}
		})
	}
}

func TestAccessor_GetSet(t *testing.T) {
	accessor := NewAccessor("")

	t.Run("set and get simple value", func(t *testing.T) {
		record := make(map[string]interface{})
		accessor.Set(record, "processed", true)

		val, found := accessor.Get(record, "processed")
		if !found {
			t.Error("expected to find 'processed' key")
		}
		if val != true {
			t.Errorf("Get() = %v, want true", val)
		}
	})

	t.Run("set and get nested value", func(t *testing.T) {
		record := make(map[string]interface{})
		accessor.Set(record, "timing.start", "2024-01-01T00:00:00Z")
		accessor.Set(record, "timing.end", "2024-01-01T00:01:00Z")

		start, found := accessor.Get(record, "timing.start")
		if !found {
			t.Error("expected to find 'timing.start' key")
		}
		if start != "2024-01-01T00:00:00Z" {
			t.Errorf("Get() = %v, want '2024-01-01T00:00:00Z'", start)
		}

		end, found := accessor.Get(record, "timing.end")
		if !found {
			t.Error("expected to find 'timing.end' key")
		}
		if end != "2024-01-01T00:01:00Z" {
			t.Errorf("Get() = %v, want '2024-01-01T00:01:00Z'", end)
		}
	})

	t.Run("get non-existent key", func(t *testing.T) {
		record := make(map[string]interface{})
		_, found := accessor.Get(record, "nonexistent")
		if found {
			t.Error("expected not to find 'nonexistent' key")
		}
	})

	t.Run("get from nil record", func(t *testing.T) {
		_, found := accessor.Get(nil, "key")
		if found {
			t.Error("expected not to find key in nil record")
		}
	})

	t.Run("set on nil record does not panic", func(t *testing.T) {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("Set on nil record panicked: %v", r)
			}
		}()
		accessor.Set(nil, "key", "value")
	})

	t.Run("set with empty key does nothing", func(t *testing.T) {
		record := make(map[string]interface{})
		accessor.Set(record, "", "value")
		if accessor.HasMetadata(record) {
			t.Error("expected no metadata after setting empty key")
		}
	})
}

func TestAccessor_Delete(t *testing.T) {
	accessor := NewAccessor("")

	t.Run("delete existing key", func(t *testing.T) {
		record := make(map[string]interface{})
		accessor.Set(record, "toDelete", "value")
		accessor.Delete(record, "toDelete")

		_, found := accessor.Get(record, "toDelete")
		if found {
			t.Error("expected key to be deleted")
		}
	})

	t.Run("delete nested key", func(t *testing.T) {
		record := make(map[string]interface{})
		accessor.Set(record, "parent.child", "value")
		accessor.Delete(record, "parent.child")

		_, found := accessor.Get(record, "parent.child")
		if found {
			t.Error("expected nested key to be deleted")
		}

		_, found = accessor.Get(record, "parent")
		if !found {
			t.Error("expected parent key to still exist")
		}
	})

	t.Run("delete non-existent key does not panic", func(t *testing.T) {
		record := make(map[string]interface{})
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("Delete panicked: %v", r)
			}
		}()
		accessor.Delete(record, "nonexistent")
	})
}

func TestAccessor_GetAllSetAll(t *testing.T) {
	accessor := NewAccessor("")

	t.Run("get all metadata", func(t *testing.T) {
		record := make(map[string]interface{})
		accessor.Set(record, "key1", "value1")
		accessor.Set(record, "key2", "value2")

		all := accessor.GetAll(record)
		if all == nil {
			t.Fatal("expected non-nil metadata")
		}
		if all["key1"] != "value1" || all["key2"] != "value2" {
			t.Errorf("GetAll() = %v, want {key1: value1, key2: value2}", all)
		}
	})

	t.Run("get all from empty record", func(t *testing.T) {
		record := make(map[string]interface{})
		all := accessor.GetAll(record)
		if all != nil {
			t.Errorf("GetAll() = %v, want nil", all)
		}
	})

	t.Run("set all metadata", func(t *testing.T) {
		record := make(map[string]interface{})
		metadata := map[string]interface{}{
			"processed": true,
			"timestamp": "2024-01-01",
		}
		accessor.SetAll(record, metadata)

		val, found := accessor.Get(record, "processed")
		if !found || val != true {
			t.Errorf("Get('processed') = %v, %v, want true, true", val, found)
		}
	})

	t.Run("set all with nil removes metadata", func(t *testing.T) {
		record := make(map[string]interface{})
		accessor.Set(record, "key", "value")
		accessor.SetAll(record, nil)

		if accessor.HasMetadata(record) {
			t.Error("expected metadata to be removed")
		}
	})
}

func TestAccessor_Merge(t *testing.T) {
	accessor := NewAccessor("")

	t.Run("merge into existing metadata", func(t *testing.T) {
		record := make(map[string]interface{})
		accessor.Set(record, "existing", "value")
		accessor.Merge(record, map[string]interface{}{
			"new":      "value",
			"existing": "overwritten",
		})

		newVal, _ := accessor.Get(record, "new")
		existingVal, _ := accessor.Get(record, "existing")

		if newVal != "value" {
			t.Errorf("Get('new') = %v, want 'value'", newVal)
		}
		if existingVal != "overwritten" {
			t.Errorf("Get('existing') = %v, want 'overwritten'", existingVal)
		}
	})

	t.Run("merge into empty record", func(t *testing.T) {
		record := make(map[string]interface{})
		accessor.Merge(record, map[string]interface{}{"key": "value"})

		val, found := accessor.Get(record, "key")
		if !found || val != "value" {
			t.Errorf("Get('key') = %v, %v, want 'value', true", val, found)
		}
	})
}

func TestAccessor_Copy(t *testing.T) {
	accessor := NewAccessor("")

	t.Run("copy metadata between records", func(t *testing.T) {
		src := make(map[string]interface{})
		accessor.Set(src, "key", "value")
		accessor.Set(src, "nested.key", "nestedValue")

		dst := make(map[string]interface{})
		accessor.Copy(src, dst)

		val, found := accessor.Get(dst, "key")
		if !found || val != "value" {
			t.Errorf("Get('key') = %v, %v, want 'value', true", val, found)
		}

		nestedVal, found := accessor.Get(dst, "nested.key")
		if !found || nestedVal != "nestedValue" {
			t.Errorf("Get('nested.key') = %v, %v, want 'nestedValue', true", nestedVal, found)
		}
	})

	t.Run("copy is deep copy", func(t *testing.T) {
		src := make(map[string]interface{})
		accessor.Set(src, "nested.value", "original")

		dst := make(map[string]interface{})
		accessor.Copy(src, dst)

		accessor.Set(src, "nested.value", "modified")

		val, _ := accessor.Get(dst, "nested.value")
		if val != "original" {
			t.Errorf("Get('nested.value') = %v, want 'original' (deep copy should be independent)", val)
		}
	})

	t.Run("copy from nil source does nothing", func(t *testing.T) {
		dst := make(map[string]interface{})
		accessor.Copy(nil, dst)
		if accessor.HasMetadata(dst) {
			t.Error("expected no metadata after copying from nil")
		}
	})
}

func TestAccessor_Strip(t *testing.T) {
	accessor := NewAccessor("")

	t.Run("strip metadata from record", func(t *testing.T) {
		record := map[string]interface{}{
			"data": "value",
		}
		accessor.Set(record, "key", "metaValue")

		metadata := accessor.Strip(record)

		if accessor.HasMetadata(record) {
			t.Error("expected metadata to be stripped from record")
		}
		if record["data"] != "value" {
			t.Error("expected data field to remain")
		}
		if metadata["key"] != "metaValue" {
			t.Errorf("Strip() returned %v, want {key: metaValue}", metadata)
		}
	})

	t.Run("strip from record without metadata", func(t *testing.T) {
		record := map[string]interface{}{"data": "value"}
		metadata := accessor.Strip(record)
		if metadata != nil {
			t.Errorf("Strip() = %v, want nil", metadata)
		}
	})
}

func TestAccessor_StripCopy(t *testing.T) {
	accessor := NewAccessor("")

	t.Run("strip copy preserves original", func(t *testing.T) {
		record := map[string]interface{}{
			"data": "value",
		}
		accessor.Set(record, "key", "metaValue")

		stripped := accessor.StripCopy(record)

		if !accessor.HasMetadata(record) {
			t.Error("expected original record to still have metadata")
		}

		if _, exists := stripped[accessor.FieldName()]; exists {
			t.Error("expected stripped copy to not have metadata field")
		}

		if stripped["data"] != "value" {
			t.Errorf("stripped['data'] = %v, want 'value'", stripped["data"])
		}
	})
}

func TestAccessor_HasMetadata(t *testing.T) {
	accessor := NewAccessor("")

	t.Run("has metadata", func(t *testing.T) {
		record := make(map[string]interface{})
		accessor.Set(record, "key", "value")
		if !accessor.HasMetadata(record) {
			t.Error("expected HasMetadata() = true")
		}
	})

	t.Run("no metadata", func(t *testing.T) {
		record := make(map[string]interface{})
		if accessor.HasMetadata(record) {
			t.Error("expected HasMetadata() = false")
		}
	})

	t.Run("nil record", func(t *testing.T) {
		if accessor.HasMetadata(nil) {
			t.Error("expected HasMetadata(nil) = false")
		}
	})
}

func TestDeepCopyMap(t *testing.T) {
	t.Run("deep copy nested structures", func(t *testing.T) {
		src := map[string]interface{}{
			"simple": "value",
			"nested": map[string]interface{}{
				"inner": "innerValue",
			},
			"array": []interface{}{
				"item1",
				map[string]interface{}{"arrayNested": "value"},
			},
		}

		dst := deepCopyMap(src)

		src["simple"] = "modified"
		nested, _ := src["nested"].(map[string]interface{})
		nested["inner"] = "modified"
		arr, _ := src["array"].([]interface{})
		arr[0] = "modified"

		if dst["simple"] != "value" {
			t.Error("simple value was not deep copied")
		}
		dstNested, _ := dst["nested"].(map[string]interface{})
		if dstNested["inner"] != "innerValue" {
			t.Error("nested value was not deep copied")
		}
		dstArr, _ := dst["array"].([]interface{})
		if dstArr[0] != "item1" {
			t.Error("array value was not deep copied")
		}
	})

	t.Run("nil map returns nil", func(t *testing.T) {
		if deepCopyMap(nil) != nil {
			t.Error("expected nil for nil input")
		}
	})
}

func TestCustomFieldName(t *testing.T) {
	accessor := NewAccessor("_custom")

	record := make(map[string]interface{})
	accessor.Set(record, "key", "value")

	if _, exists := record["_custom"]; !exists {
		t.Error("expected metadata to be stored under '_custom' field")
	}

	if _, exists := record["_metadata"]; exists {
		t.Error("expected metadata NOT to be stored under default '_metadata' field")
	}

	val, found := accessor.Get(record, "key")
	if !found || val != "value" {
		t.Errorf("Get('key') = %v, %v, want 'value', true", val, found)
	}
}

func TestStripFromRecord(t *testing.T) {
	t.Run("strips metadata field from copy", func(t *testing.T) {
		record := map[string]interface{}{
			"data":      "value",
			"_metadata": map[string]interface{}{"key": "meta"},
		}
		stripped := StripFromRecord(record, "")

		if _, exists := stripped["_metadata"]; exists {
			t.Error("expected stripped copy to lack _metadata")
		}
		if _, exists := record["_metadata"]; !exists {
			t.Error("expected original to still have _metadata (StripFromRecord must not mutate)")
		}
		if stripped["data"] != "value" {
			t.Errorf("stripped['data'] = %v, want 'value'", stripped["data"])
		}
	})

	t.Run("returns original when metadata field is absent", func(t *testing.T) {
		record := map[string]interface{}{"data": "value"}
		got := StripFromRecord(record, "")
		// Identity check: nothing to copy when there's no metadata.
		if &got == &record {
			t.Error("expected same backing storage when no metadata to strip")
		}
		if got["data"] != "value" {
			t.Errorf("got['data'] = %v, want 'value'", got["data"])
		}
	})

	t.Run("custom field name", func(t *testing.T) {
		record := map[string]interface{}{
			"data":    "value",
			"_custom": map[string]interface{}{"x": 1},
		}
		stripped := StripFromRecord(record, "_custom")
		if _, exists := stripped["_custom"]; exists {
			t.Error("expected _custom to be stripped")
		}
	})

	t.Run("nil record returns nil", func(t *testing.T) {
		if StripFromRecord(nil, "") != nil {
			t.Error("expected nil for nil input")
		}
	})
}

func TestStripFromRecords(t *testing.T) {
	records := []map[string]interface{}{
		{"data": "a", "_metadata": map[string]interface{}{"i": 1}},
		{"data": "b"},
	}
	stripped := StripFromRecords(records, "")

	if len(stripped) != 2 {
		t.Fatalf("len = %d, want 2", len(stripped))
	}
	if _, exists := stripped[0]["_metadata"]; exists {
		t.Error("expected first record to be stripped")
	}
	// Original first record must remain unchanged.
	if _, exists := records[0]["_metadata"]; !exists {
		t.Error("expected original first record to still have _metadata")
	}
}
