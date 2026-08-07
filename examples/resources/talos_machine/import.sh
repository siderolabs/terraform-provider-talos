# Import records identity (id/node/endpoint). The import ID must match HCL
# `node` exactly (RequiresReplace). Expect a follow-up plan/apply that fills
# client_configuration and other attributes from HCL (typically from
# talos_machine_secrets already in state). See the resource docs Import section.
terraform import talos_machine.example 10.5.0.2
