function transform(record) {
  var email = String(record.email || "").trim().toLowerCase();
  record.email = email;
  record.emailDomain = email.indexOf("@") >= 0 ? email.split("@")[1] : "";
  record.hasMarketingConsent = record.marketingConsent === true || record.marketingConsent === "true";
  return record;
}
