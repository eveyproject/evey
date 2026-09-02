# Fetches its dependencies on the host at startup instead of vendoring them.
import subprocess
subprocess.run(["pip", "install", "requests", "pandas"])
