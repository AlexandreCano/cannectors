package runtime

import (
	"testing"
)

func TestNewMetadataAccessor(t *testing.T) {
	tests := []struct {
		name          string
		fieldName     string
		wantFieldName string
	}{
		{
			name:          "empty uses default",
			fieldName:     "",
			wantFieldName: DefaultMetadataFieldName,
		},
		{
			name:          "custom field name",
			fieldName:     "_custom",
			wantFieldName: "_custom",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			accessor := NewMetadataAccessor(tt.fieldName)
			if accessor.FieldName() != tt.wantFieldName {
				t.Errorf("FieldName() = %q, want %q", accessor.FieldName(), tt.wantFieldName)
			}
		})
	}
}

func TestMetadataAccessor_GetSet(t *testing.T) {
	accessor := NewMetadataAccessor("")

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

func TestMetadataAccessor_Delete(t *testing.T) {
	accessor := NewMetadataAccessor("")

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

		// Parent should still exist
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

func TestMetadataAccessor_GetAllSetAll(t *testing.T) {
	accessor := NewMetadataAccessor("")

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

func TestMetadataAccessor_Merge(t *testing.T) {
	accessor := NewMetadataAccessor("")

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

func TestMetadataAccessor_Copy(t *testing.T) {
	accessor := NewMetadataAccessor("")

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

		// Modify source
		accessor.Set(src, "nested.value", "modified")

		// Destination should be unchanged
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

func TestMetadataAccessor_Strip(t *testing.T) {
	accessor := NewMetadataAccessor("")

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

func TestMetadataAccessor_StripCopy(t *testing.T) {
	accessor := NewMetadataAccessor("")

	t.Run("strip copy preserves original", func(t *testing.T) {
		record := map[string]interface{}{
			"data": "value",
		}
		accessor.Set(record, "key", "metaValue")

		stripped := accessor.StripCopy(record)

		// Original should still have metadata
		if !accessor.HasMetadata(record) {
			t.Error("expected original record to still have metadata")
		}

		// Stripped should not have metadata
		if _, exists := stripped[accessor.FieldName()]; exists {
			t.Error("expected stripped copy to not have metadata field")
		}

		// Stripped should have data
		if stripped["data"] != "value" {
			t.Errorf("stripped['data'] = %v, want 'value'", stripped["data"])
		}
	})
}

func TestMetadataAccessor_HasMetadata(t *testing.T) {
	accessor := NewMetadataAccessor("")

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

		// Modify source (safe type assertions for test data)
		src["simple"] = "modified"
		nested, _ := src["nested"].(map[string]interface{})
		nested["inner"] = "modified"
		arr, _ := src["array"].([]interface{})
		arr[0] = "modified"

		// Destination should be unchanged
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

func TestCustomMetadataFieldName(t *testing.T) {
	accessor := NewMetadataAccessor("_custom")

	record := make(map[string]interface{})
	accessor.Set(record, "key", "value")

	// Check that metadata is stored under custom field name
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
