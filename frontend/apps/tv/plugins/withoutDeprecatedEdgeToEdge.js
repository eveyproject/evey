const {
  withAppBuildGradle,
  withDangerousMod,
  withGradleProperties,
} = require("expo/config-plugins");
const fs = require("node:fs");
const path = require("node:path");

// Android 15 deprecated the system bar colour calls and the DEFAULT and SHORT_EDGES display
// cutout modes. React Native, androidx and Material still use them, so they are removed from
// the release bytecode. Same treatment as the phone app (apps/mobile/android/app), whose
// android folder is tracked while this one is generated.
const MARK = "evey: no deprecated edge-to-edge calls (withoutDeprecatedEdgeToEdge)";

const RULES = `# --- ${MARK} ---
-assumenosideeffects class android.view.Window {
    public void setStatusBarColor(int);
    public void setNavigationBarColor(int);
    public int getStatusBarColor() return 0;
}
`;

const IMPORTS = `import com.android.build.api.instrumentation.AsmClassVisitorFactory
import com.android.build.api.instrumentation.ClassContext
import com.android.build.api.instrumentation.ClassData
import com.android.build.api.instrumentation.InstrumentationParameters
import com.android.build.api.instrumentation.InstrumentationScope
import org.objectweb.asm.ClassVisitor
import org.objectweb.asm.MethodVisitor
import org.objectweb.asm.Opcodes

`;

const TRANSFORM = `
// --- ${MARK} ---
abstract class DropCutoutModeWrites implements AsmClassVisitorFactory<InstrumentationParameters.None> {
    @Override
    ClassVisitor createClassVisitor(ClassContext classContext, ClassVisitor next) {
        return new ClassVisitor(Opcodes.ASM9, next) {
            @Override
            MethodVisitor visitMethod(int access, String name, String descriptor, String signature, String[] exceptions) {
                MethodVisitor mv = super.visitMethod(access, name, descriptor, signature, exceptions)
                return new MethodVisitor(Opcodes.ASM9, mv) {
                    @Override
                    void visitFieldInsn(int opcode, String owner, String field, String fieldDescriptor) {
                        if (opcode == Opcodes.PUTFIELD && owner == 'android/view/WindowManager$LayoutParams' && field == 'layoutInDisplayCutoutMode') {
                            super.visitInsn(Opcodes.POP2)
                        } else {
                            super.visitFieldInsn(opcode, owner, field, fieldDescriptor)
                        }
                    }
                }
            }
        }
    }

    @Override
    boolean isInstrumentable(ClassData classData) {
        return ['com.facebook.react.views.view.WindowUtil', 'androidx.core.view.WindowCompat', 'androidx.activity.EdgeToEdge', 'com.google.android.material.internal.EdgeToEdgeUtils'].any { classData.className.startsWith(it) }
    }
}

androidComponents {
    onVariants(selector().all()) { variant ->
        variant.instrumentation.transformClassesWith(DropCutoutModeWrites, InstrumentationScope.ALL) {}
    }
}
`;

function withoutDeprecatedEdgeToEdge(config) {
  config = withGradleProperties(config, (cfg) => {
    const key = "android.r8.optimizedResourceShrinking";
    const existing = cfg.modResults.find((p) => p.type === "property" && p.key === key);
    if (existing) existing.value = "true";
    else cfg.modResults.push({ type: "property", key, value: "true" });
    return cfg;
  });
  config = withAppBuildGradle(config, (cfg) => {
    if (!cfg.modResults.contents.includes(MARK)) {
      cfg.modResults.contents = `${IMPORTS}${cfg.modResults.contents.replace(/\n*$/, "\n")}${TRANSFORM}`;
    }
    return cfg;
  });
  return withDangerousMod(config, [
    "android",
    async (cfg) => {
      const file = path.join(cfg.modRequest.platformProjectRoot, "app", "proguard-rules.pro");
      const current = fs.existsSync(file) ? fs.readFileSync(file, "utf8") : "";
      if (!current.includes(MARK)) fs.writeFileSync(file, `${current.replace(/\n*$/, "\n")}\n${RULES}`);
      return cfg;
    },
  ]);
}

module.exports = withoutDeprecatedEdgeToEdge;
module.exports.MARK = MARK;
