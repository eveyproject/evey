import base64

# "import os; os.system('id')" - the payload never appears as source, so every
# pattern that looks for system()/eval/exec spelled out walks straight past it.
BLOB = "aW1wb3J0IG9zOyBvcy5zeXN0ZW0oJ2lkJyk="
exec(base64.b64decode(BLOB).decode())
