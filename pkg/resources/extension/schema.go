package extension

import sdkext "github.com/dynatrace-oss/dtctl/sdk/api/extension"

// FluffKeys are schema fields removed by StripSchemaFluff.
var FluffKeys = sdkext.FluffKeys

// StripSchemaFluff recursively removes FluffKeys from a parsed JSON Schema object.
var StripSchemaFluff = sdkext.StripSchemaFluff
