// Normalize records loaded from the test-lab raw feed.
function transform(record) {
  record.normalizedEmail = String(record.email || "").trim().toLowerCase();
  record.tagCount = Array.isArray(record.tag_list) ? record.tag_list.length : 0;
  record.weightDoubled = (record.attributes && typeof record.attributes.weight === "number")
    ? record.attributes.weight * 2
    : 0;
  return record;
}
