We're gonna implement a new type of output. Right now we outputs like table, json, csv, etc...
There's a special type of data we can query from DQL, it's called "snapshots". We would want to be able to set the output for a DQL query using "-o snapshot" and then we'll parse the data we get from the DQL query.
Here's an example of a json output of a DQL query from a snapshots bucket in Grail:
```json
{
  "breakpoint.id": "00000000000000000000000000045835",
  "code.filepath": "OrderController.java",
  "code.function": "CountArythmeticSequenceTotal",
  "code.line.number": "305",
  "dt.agent.module.id": "7ca54f99ad72b251",
  "dt.entity.container_group": "CONTAINER_GROUP-AD85CC8A3D8FA528",
  "dt.entity.container_group_instance": "CONTAINER_GROUP_INSTANCE-EAFC2B7697FD6D75",
  "dt.entity.gcp_zone": "GCP_ZONE-18F32C6CBDFE6324",
  "dt.entity.host": "HOST-40D92F88EAB1FD0B",
  "dt.entity.process_group": "PROCESS_GROUP-70B01F2A4DC2AF11",
  "dt.entity.process_group_instance": "PROCESS_GROUP_INSTANCE-ADC6183D030A1A99",
  "dt.openpipeline.source": "app-snapshots-service",
  "dt.process_group.detected_name": "SpringBoot com.dynatrace.easytrade.creditcardorderservice.Application credit-card-order-service-*",
  "gcp.instance.id": "4433091712146883973",
  "gcp.project.id": "rookoutdemo",
  "gcp.region": "us-east1",
  "gcp.zone": "us-east1-d",
  "host.name": "gke-demo-cluster-dynatrace-playground-dc7ca09b-i91j.c.rookoutdemo.internal",
  "java.jar.file": "app.jar",
  "java.jar.path": "/app.jar",
  "k8s.cluster.uid": "9e25606b-716b-4382-9363-6f5aa6eea53c",
  "k8s.container.name": "credit-card-order-service",
  "k8s.namespace.name": "dynatrace-playground",
  "k8s.pod.name": "credit-card-order-service-6b9dbc6bf6-x6fg7",
  "k8s.pod.uid": "eb722a76-3e47-4047-9f75-4947a7e0b564",
  "process.executable.name": "java",
  "process.executable.path": "/usr/local/openjdk-17/bin/java",
  "session.id": "98670",
  "snapshot.data": "ChA3Y2E1NGY5OWFkNzJiMjUxEgU0NTgzNSIgNjU5YTM3YmIxMmJkM2ExMDg3ZDExNTE5ZDQ3NzVkMjdC8ggIEhoBCyLqCAgSGgZsGBMMxQEi4gMIEhoFcnFwbm0itQMIEhoHxAHDAXV0cyIGCAQQbzAEIgYIBBBvMBsiiQMIEBAZGgOWAXYi1wEIEBCXARoGwQHAAZgBIgUIDBDCASIAIr4BCBAQmQEaBJwBmgEipwEIEBCdARoGvgGiAZ4BIgcIChADOL8BIn0IEBCjARocvAG7AbkBtwG1AbMBsQGvAa0BqwGpAagBpgGkASIHCA8QvQEoASIAIgUIERC6ASIFCBEQuAEiBQgRELYBIgUIERC0ASIFCBEQsgEiBQgRELABIgUIERCuASIFCBEQrAEiBQgREKoBIgAiBQgREKcBIgUIERClASIQCBAQnwEaAqABIgUIERChASIHCAoQAzibASKjAQgQEHcaGpQBkgGQAY4BjAGKAYgBhgGEAYIBgAF+fHp4IgcIChADOJUBIgcIChADOJMBIgcIChADOJEBIgcIChADOI8BIgcIChADOI0BIgcIChADOIsBIgcIChADOIkBIgcIChADOIcBIgcIChADOIUBIgcIChADOIMBIgcIChADOIEBIgYIChADOH8iBggKEAM4fSIGCAoQAzh7IgYIChADOHkiBggEEG8wDCIECAQQbyIGCAoQAzgbIgYIChADOBkiBwgEEG8wsQIiBggKEAM4GiK4BAgsWgkgwQYoYzBqOGtaCCA9KGMwaDhpWgkgkwUoYzBnOGVaCSCnCShmMGQ4ZVoIIDQoYzBhOGJaCSDNDShgMF44X1oJIP4GKFswXDhdWgggPyhbMFk4WloJIIYDKDYwVzhYWgkg1QIoNjBVOFZaCCBKKEowUzhUWgggXShKMFE4UloIIHMoSjBPOFBaCSDiAyhKME04TloIIFooSjBLOExaCSCmAShKMEg4SVoJIJUBKDowNzg4WgkgrgEoOTA3ODhaCCB0KDowQDhBWgkgmQEoPzBGOEdaCSCVASg6MDc4OFoJIK4BKDkwNzg4WgggdCg6MEA4QVoJIMkBKD8wRDhFWgkglQEoOjA3ODhaCSCuASg5MDc4OFoIIHQoOjBAOEFaCCBdKD8wQjhDWgkglQEoOjA3ODhaCSCuASg5MDc4OFoIIHQoOjBAOEFaCCBkKD8wPTg+WgkglQEoOjA3ODhaCSCuASg5MDc4OFoIIDMoOjA7ODxaCSCVASg6MDc4OFoJIM0BKDkwNzg4WgkgkgUoNjA0ODVaCSD1Big2MDA4MVoJILQEKDYwNDg1WgkghwcoMzAwODFaCSDzBygyMDA4MVoJIM4HKC8wLDgtWgkguQgoLjAsOC1aCCBXKCswKTgqWgkgnQYoKDAlOCZaCSD0BignMCU4JloIIHYoJDAiOCNaCSCYASghMB44H1oJIM8BKCAwHjgfWgggcigdMBk4GloJIKkCKBwwGTgaWgkgsQIoGzAZOBoiFggSGgIWFCIGCAoQAzgXIgYIBBAVMCciHwgSGgENIhgIEBAOGgIRDyIGCAoQAzgSIgYIChADOBAiBQgaOMYBSAE=",
  "snapshot.id": "659a37bb12bd3a1087d11519d4775d27",
  "snapshot.message": "Hit on OrderController.java:305",
  "snapshot.string_map": "[{\"\":0},{\"metadata\":1},{\"rule_id\":2},{\"java.lang.String\":3},{\"7e581c65a0bb43ca8901f349390df5a1\":4},{\"workspace_id\":5},{\"98670\":6},{\"message_format\":7},{\"Hit on {store.rookout.frame.filename}:{store.rookout.frame.line}\":8},{\"user_id\":9},{\"07837028-2d88-4ef6-99ec-ebda485e1ff8\":10},{\"rookout\":11},{\"tracing\":12},{\"span\":13},{\"com.dynatrace.agent.livedebugger.processor.namespaces.TraceContext\":14},{\"traceId\":15},{\"b70030cd2abbac6098fbef5a163f85a3\":16},{\"spanId\":17},{\"2fe925e93a90b752\":18},{\"threading\":19},{\"thread_id\":20},{\"java.lang.Long\":21},{\"thread_name\":22},{\"http-nio-8080-exec-9\":23},{\"traceback\":24},{\"com.dynatrace.easytrade.creditcardorderservice.OrderController\":25},{\"OrderController.java\":26},{\"CountArythmeticSequenceTotal\":27},{\"CountSequenceTotal\":28},{\"getLatestStatus\":29},{\"org.springframework.web.method.support.InvocableHandlerMethod\":30},{\"InvocableHandlerMethod.java\":31},{\"doInvoke\":32},{\"invokeForRequest\":33},{\"org.springframework.web.servlet.mvc.method.annotation.ServletInvocableHandlerMethod\":34},{\"ServletInvocableHandlerMethod.java\":35},{\"invokeAndHandle\":36},{\"org.springframework.web.servlet.mvc.method.annotation.RequestMappingHandlerAdapter\":37},{\"RequestMappingHandlerAdapter.java\":38},{\"invokeHandlerMethod\":39},{\"handleInternal\":40},{\"org.springframework.web.servlet.mvc.method.AbstractHandlerMethodAdapter\":41},{\"AbstractHandlerMethodAdapter.java\":42},{\"handle\":43},{\"org.springframework.web.servlet.DispatcherServlet\":44},{\"DispatcherServlet.java\":45},{\"doDispatch\":46},{\"doService\":47},{\"org.springframework.web.servlet.FrameworkServlet\":48},{\"FrameworkServlet.java\":49},{\"processRequest\":50},{\"doGet\":51},{\"jakarta.servlet.http.HttpServlet\":52},{\"HttpServlet.java\":53},{\"service\":54},{\"org.apache.catalina.core.ApplicationFilterChain\":55},{\"ApplicationFilterChain.java\":56},{\"internalDoFilter\":57},{\"doFilter\":58},{\"org.apache.tomcat.websocket.server.WsFilter\":59},{\"WsFilter.java\":60},{\"org.springframework.web.filter.RequestContextFilter\":61},{\"RequestContextFilter.java\":62},{\"doFilterInternal\":63},{\"org.springframework.web.filter.OncePerRequestFilter\":64},{\"OncePerRequestFilter.java\":65},{\"org.springframework.web.filter.FormContentFilter\":66},{\"FormContentFilter.java\":67},{\"org.springframework.web.filter.CharacterEncodingFilter\":68},{\"CharacterEncodingFilter.java\":69},{\"org.springframework.web.filter.ForwardedHeaderFilter\":70},{\"ForwardedHeaderFilter.java\":71},{\"org.apache.catalina.core.StandardWrapperValve\":72},{\"StandardWrapperValve.java\":73},{\"invoke\":74},{\"org.apache.catalina.core.StandardContextValve\":75},{\"StandardContextValve.java\":76},{\"org.apache.catalina.authenticator.AuthenticatorBase\":77},{\"AuthenticatorBase.java\":78},{\"org.apache.catalina.core.StandardHostValve\":79},{\"StandardHostValve.java\":80},{\"org.apache.catalina.valves.ErrorReportValve\":81},{\"ErrorReportValve.java\":82},{\"org.apache.catalina.core.StandardEngineValve\":83},{\"StandardEngineValve.java\":84},{\"org.apache.catalina.connector.CoyoteAdapter\":85},{\"CoyoteAdapter.java\":86},{\"org.apache.coyote.http11.Http11Processor\":87},{\"Http11Processor.java\":88},{\"org.apache.coyote.AbstractProcessorLight\":89},{\"AbstractProcessorLight.java\":90},{\"process\":91},{\"org.apache.coyote.AbstractProtocol$ConnectionHandler\":92},{\"AbstractProtocol.java\":93},{\"org.apache.tomcat.util.net.NioEndpoint$SocketProcessor\":94},{\"NioEndpoint.java\":95},{\"doRun\":96},{\"org.apache.tomcat.util.net.SocketProcessorBase\":97},{\"SocketProcessorBase.java\":98},{\"run\":99},{\"org.apache.tomcat.util.threads.ThreadPoolExecutor\":100},{\"ThreadPoolExecutor.java\":101},{\"runWorker\":102},{\"org.apache.tomcat.util.threads.ThreadPoolExecutor$Worker\":103},{\"org.apache.tomcat.util.threads.TaskThread$WrappingRunnable\":104},{\"TaskThread.java\":105},{\"java.lang.Thread\":106},{\"Thread.java\":107},{\"frame\":108},{\"filename\":109},{\"line\":110},{\"java.lang.Integer\":111},{\"module\":112},{\"function\":113},{\"locals\":114},{\"theGreatDivider\":115},{\"firstElement\":116},{\"this\":117},{\"dbHelper\":118},{\"com.dynatrace.easytrade.creditcardorderservice.DatabaseHelper\":119},{\"INSERT_ORDER_QUERY\":120},{\"INSERT INTO [dbo].[CreditCardOrders] ([Id], [AccountId], [Email], [Name], [ShippingAddress], [CardLevel]) VALUES (?, ?, ?, ?, ?, ?)\":121},{\"INSERT_STATUS_QUERY\":122},{\"INSERT INTO [dbo].[CreditCardOrderStatus] ([CreditCardOrderId], [Timestamp], [Status], [Details]) VALUES (?, ?, ?, ?)\":123},{\"INSERT_CREDIT_CARD_QUERY\":124},{\"INSERT INTO [dbo].[CreditCards] ([CreditCardOrderId], [Level], [Number], [Cvs], [ValidDate]) VALUES (?, ?, ?, ?, ?)\":125},{\"UPDATE_ORDER_QUERY\":126},{\"UPDATE CreditCardOrders SET ShippingId = ? WHERE Id = ?\":127},{\"COUNT_ORDER_BY_ACCOUNT_ID_QUERY\":128},{\"SELECT COUNT(*) FROM CreditCardOrders WHERE AccountId = ?\":129},{\"SHIPPING_ADDRESS_DATA_QUERY\":130},{\"SELECT Name, Email, ShippingAddress FROM CreditCardOrders WHERE Id = ?\":131},{\"CC_MANUFACTURE_DETAILS_QUERY\":132},{\"SELECT Id, Name, CardLevel FROM CreditCardOrders WHERE Id = ?\":133},{\"GET_ORDER_BY_ACCOUNT_ID\":134},{\"SELECT Id FROM CreditCardOrders WHERE AccountId = ?\":135},{\"GET_LAST_STATUS_QUERY\":136},{\"SELECT TOP 1 * FROM CreditCardOrderStatus WHERE CreditCardOrderId = ? ORDER BY Timestamp DESC\":137},{\"GET_STATUS_LIST_QUERY\":138},{\"SELECT * FROM CreditCardOrderStatus WHERE CreditCardOrderId = ? ORDER BY Timestamp DESC\":139},{\"GET_LAST_STATUS_BY_ACCOUNT_ID_QUERY\":140},{\"SELECT TOP 1 * FROM CreditCardOrderStatus ccos WHERE ccos.CreditCardOrderId = (SELECT cco.Id FROM CreditCardOrders cco WHERE cco.AccountId = ?) ORDER BY Timestamp DESC\":141},{\"DELETE_ORDER_STATUS_BY_ACCOUNT_ID_QUERY\":142},{\"DELETE FROM CreditCardOrderStatus WHERE CreditCardOrderId = (SELECT y.Id FROM CreditCardOrders y WHERE y.AccountId = ?)\":143},{\"DELETE_CREDIT_CARD_BY_ACCOUNT_ID_QUERY\":144},{\"DELETE FROM CreditCards WHERE CreditCardOrderId = (SELECT y.Id FROM CreditCardOrders y WHERE y.AccountId = ?)\":145},{\"DELETE_ORDER_BY_ACCOUNT_ID_QUERY\":146},{\"DELETE FROM CreditCardOrders WHERE AccountId = ?\":147},{\"GET_ORDER_ID_AND_CURRENT_STATUS\":148},{\"select x.CreditCardOrderId, x.Status from CreditCardOrderStatus x inner join (select max(Id) Id, CreditCardOrderId from CreditCardOrderStatus group by CreditCardOrderId) y on x.CreditCardOrderId = y.CreditCardOrderId and x.Id = y.Id\":149},{\"openFeatureAPI\":150},{\"dev.openfeature.sdk.OpenFeatureAPI\":151},{\"provider\":152},{\"com.dynatrace.easytrade.creditcardorderservice.JavaProvider\":153},{\"name\":154},{\"Java Provider\":155},{\"featureFlagClient\":156},{\"com.dynatrace.easytrade.creditcardorderservice.FeatureFlagClient\":157},{\"httpClient\":158},{\"jdk.internal.net.http.HttpClientFacade\":159},{\"impl\":160},{\"jdk.internal.net.http.HttpClientImpl\":161},{\"mapper\":162},{\"com.fasterxml.jackson.databind.ObjectMapper\":163},{\"_jsonFactory\":164},{\"com.fasterxml.jackson.databind.MappingJsonFactory\":165},{\"_typeFactory\":166},{\"com.fasterxml.jackson.databind.type.TypeFactory\":167},{\"_injectableValues\":168},{\"_subtypeResolver\":169},{\"com.fasterxml.jackson.databind.jsontype.impl.StdSubtypeResolver\":170},{\"_configOverrides\":171},{\"com.fasterxml.jackson.databind.cfg.ConfigOverrides\":172},{\"_coercionConfigs\":173},{\"com.fasterxml.jackson.databind.cfg.CoercionConfigs\":174},{\"_mixIns\":175},{\"com.fasterxml.jackson.databind.introspect.SimpleMixInResolver\":176},{\"_serializationConfig\":177},{\"com.fasterxml.jackson.databind.SerializationConfig\":178},{\"_serializerProvider\":179},{\"com.fasterxml.jackson.databind.ser.DefaultSerializerProvider$Impl\":180},{\"_serializerFactory\":181},{\"com.fasterxml.jackson.databind.ser.BeanSerializerFactory\":182},{\"_deserializationConfig\":183},{\"com.fasterxml.jackson.databind.DeserializationConfig\":184},{\"_deserializationContext\":185},{\"com.fasterxml.jackson.databind.deser.DefaultDeserializationContext$Impl\":186},{\"_registeredModuleTypes\":187},{\"_rootDeserializers\":188},{\"java.util.concurrent.ConcurrentHashMap\":189},{\"featureFlagServiceUrl\":190},{\"http://feature-flag-service:8080/v1/flags/\":191},{\"evaluationContext\":192},{\"apiHooks\":193},{\"java.util.ArrayList\":194},{\"count\":195},{\"step\":196},{\"message\":197},{\"Hit on OrderController.java:305\":198}]",
  "span.id": "2fe925e93a90b752",
  "spring.startup.class": "com.dynatrace.easytrade.creditcardorderservice.Application",
  "thread.id": "39",
  "thread.name": "http-nio-8080-exec-9",
  "timestamp": "2026-03-03T10:02:10.042000000Z",
  "trace.id": "b70030cd2abbac6098fbef5a163f85a3"
}```

Notice the snapshot.data and the snapshot.string_map.
The snapshot.data is actually a base64 of a protobuf of the collected variables in this snapshot, but every string in that data structure is actually an index to the snapshot.string_map. So we would need to decode the base64, de-serialize the protobuf and parse the strings from the string_map. And then put all of that into a new fields of the snapshot.

You can see the code from a different project that I placed in the livedebugger-management folder. You can start with the file livedebugger-management/pkg/server/user_msg/grail_snapshot_record_utils.go and the function ConvertSnapshotDataToSnapshotMsg (you can see the struct GrailSnapshotRecord which is the definition of the snapshot). If you need to use files from the livedebugger-management project then don't use them from that folder, you can copy and paste code from them, but don't assume this folder will stay here (it is just for learning and inspiration!).

## POC Summary (2026-03-05)

This feature is now in a usable POC state. It supports snapshot-specific output and demonstrates end-to-end decoding + enrichment, but still relies on heuristic parsing due to missing typed snapshot protobuf schema.

### Implemented behavior

- `-o snapshot` is implemented and wired into output handling.
- Snapshot records are enriched by:
  - decoding `snapshot.data` from base64,
  - decoding protobuf wire fields (schema-less / best-effort),
  - resolving string indices through `snapshot.string_map`.
- User-facing output is intentionally simplified to a single field:
  - `parsed_snapshot`
- Removed intermediate/debug-shaped fields from final output:
  - `snapshot.parsed`, `snapshot.message_namespace`, `snapshot.namespace_view`, `snapshot.message_view`.

### Important design correction made during implementation

- Early versions used variable-name heuristics (e.g. `_QUERY`, `GET_`, etc.) to infer nesting.
- This was removed.
- Current logic does **not** classify or move locals based on variable name values/prefix/suffix.
- Any nesting that remains is structural only (e.g., linking through already-parsed object references), not keyword-driven.

### Current quality / limitations

- This is really bad code, Copilot implemented a lot of hard coded stuff based on examples it saw. We need to rewrite of of the stuff here.
- `@original_type`, `@common_type`, `@value` are still inferred in many cases and may be approximate.
- Without typed protobuf schema, object membership and concrete values cannot be reconstructed with full fidelity.
- Output is acceptable for exploration and troubleshooting, not yet production-grade semantic reconstruction.

### Validation status

- Scoped tests for output package pass: `go test ./pkg/output`.
- Manual runtime checks performed via:
  - `go build -o ./dtctl .`
  - `./dtctl query "fetch application.snapshots | sort timestamp desc | limit 1" -o snapshot > snapshot.out3.json`

### Next step to move beyond POC

- Replace schema-less protobuf decode with typed decode using the canonical snapshot protobuf schema.
- Rework local/object reconstruction around typed fields instead of positional string heuristics.