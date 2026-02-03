package scripts

// BuiltinScripts contains all pre-defined analysis scripts
var BuiltinScripts = []*Script{}

func init() {
	// Register scripts from each category
	BuiltinScripts = append(BuiltinScripts, HTTPScripts...)
	BuiltinScripts = append(BuiltinScripts, MySQLScripts...)
	BuiltinScripts = append(BuiltinScripts, PostgresScripts...)
	BuiltinScripts = append(BuiltinScripts, RedisScripts...)
	BuiltinScripts = append(BuiltinScripts, K8sScripts...)
	BuiltinScripts = append(BuiltinScripts, SecurityScripts...)
	BuiltinScripts = append(BuiltinScripts, ServiceScripts...)
	BuiltinScripts = append(BuiltinScripts, TraceScripts...)
	BuiltinScripts = append(BuiltinScripts, LogScripts...)
}
