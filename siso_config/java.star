load("@builtin//path.star", "path")
load("@builtin//struct.star", "module")

def __filegroups(ctx, vars):
    return {}

__handlers = {}

def __step_config(ctx, vars, step_config):
    if vars.use_reclient:
        step_config["rules"].extend([
            {
                "name": "g.java.d8",
                "action": "g.java.d8RE",
                "timeout": "2m",
                "use_remote_exec_wrapper": True,
            },
            {
                "name": "g.java.javac",
                "action": "g.java.javacRE",
                "timeout": "2m",
                "use_remote_exec_wrapper": True,
            },
            {
                "name": "g.java.r8",
                "action": "g.java.r8RE",
                "timeout": "2m",
                "use_remote_exec_wrapper": True,
            },
        ])
        return step_config

    java_dir = ctx.fs.canonpath(path.join(vars.JAVA_HOME, "bin"))
    javac_path = path.join(java_dir, "javac")
    javac_inputs_path = javac_path + "_remote_toolchain_inputs"
    javac_inputs = []
    if ctx.fs.exists(javac_inputs_path):
        for line in str(ctx.fs.read(javac_inputs_path)).splitlines():
            javac_inputs.append(path.join(java_dir, line))
    java_path = path.join(java_dir, "java")
    java_inputs_path = java_path + "_remote_toolchain_inputs"
    java_inputs = []
    if ctx.fs.exists(java_inputs_path):
        for line in str(ctx.fs.read(java_inputs_path)).splitlines():
            java_inputs.append(path.join(java_dir, line))

    # TODO: use phony targets for remote toolchain inputs?
    step_config["input_deps"].update({
        javac_path: javac_inputs,
        java_path: java_inputs,
    })

    if vars.use_rbe:
        step_config["rules"].extend([
            {
                "name": "g.java.d8Inc",
                "action": "g.java.d8Inc",
                "remote": True,
                "platform_ref": "java16",
                "timeout": "8m",
            },
            {
                "name": "g.java.r8",
                "action": "g.java.r8",
                "remote": True,
                "platform_ref": "java16",
                "timeout": "8m",
                "deps": "none",  # disable remote: failed to get gcc deps: failed to normalize args: unsupported commandline.
            },
            {
                "name": "g.java.javac",
                "action": "g.java.javac",
                "remote": True,
                "platform_ref": "java16",
                "timeout": "8m",
            },
            {
                "name": "m.*.metalava",
                "action": "m.*.metalava",
                "inputs": [
                    java_path,
                ],
                "remote": True,
                "platform_ref": "java16",
                "timeout": "8m",
            },
        ])
    return step_config

java = module(
    "java",
    step_config = __step_config,
    filegroups = __filegroups,
    handlers = __handlers,
)
