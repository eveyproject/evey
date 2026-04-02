#!/bin/bash
# Run from the Evey repo root:
# cd ../../ && bash plugins/python-example/generate_proto.sh
python -m grpc_tools.protoc \
  -I proto \
  --python_out=plugins/python-example \
  --grpc_python_out=plugins/python-example \
  proto/plugin/v1/plugin_api.proto
