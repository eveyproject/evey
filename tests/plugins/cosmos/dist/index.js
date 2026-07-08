"use strict";(()=>{function N(i){return i/864e5+24405875e-1}function F(i){return(N(i)-2451545)/36525}function R(i){return N(i)-24515435e-1}function E(i){return 23.4392911-.0130042*F(i)}var Ee=Math.PI/180,Le=180/Math.PI;function g(i){return i*Ee}function y(i){return i*Le}function u(i){let e=i%360;return e<0?e+360:e}function C(i,e,t){return i<e?e:i>t?t:i}var Te=2451545;function ke(i){let t=N(i)-Te,n=t/36525,a=280.46061837+360.98564736629*t+387933e-9*n*n-n*n*n/3871e4;return u(a)}function J(i,e){return u(ke(i)+e)}function Z(i,e,t,n){let a=i*15,o=u(t-a),r=g(o),s=g(e),c=g(n),l=Math.sin(s)*Math.sin(c)+Math.cos(s)*Math.cos(c)*Math.cos(r),m=Math.asin(C(l,-1,1)),b=Math.cos(m);if(b<1e-6)return{altDeg:y(m),azDeg:0};let h=C((Math.sin(s)-Math.sin(m)*Math.sin(c))/(b*Math.cos(c)),-1,1),d=-Math.sin(r)*Math.cos(s)/b,p=Math.atan2(d,h);return{altDeg:y(m),azDeg:u(y(p))}}function Q(i,e){let t=g(90-i),n=Math.tan(t/2),a=g(e);return{x:n*Math.sin(a),y:n*Math.cos(a),visible:i>-2}}var K={N0:0,Nd:0,i0:0,id:0,w0:282.9404,wd:470935e-10,a:1,e0:.016709,ed:-1151e-12,M0:356.047,Md:.9856002585},ee={N0:76.6799,Nd:24659e-9,i0:3.3946,id:275e-10,w0:54.891,wd:138374e-10,a:.72333,e0:.006773,ed:-1302e-12,M0:48.0052,Md:1.6021302244},te={N0:49.5574,Nd:211081e-10,i0:1.8497,id:-178e-10,w0:286.5016,wd:292961e-10,a:1.523688,e0:.093405,ed:2516e-12,M0:18.6021,Md:.5240207766},ne={N0:100.4542,Nd:276854e-10,i0:1.303,id:-1557e-10,w0:273.8777,wd:164505e-10,a:5.20256,e0:.048498,ed:4469e-12,M0:19.895,Md:.0830853001},ae={N0:113.6634,Nd:23898e-9,i0:2.4886,id:-1081e-10,w0:339.3939,wd:297661e-10,a:9.55475,e0:.055546,ed:-9499e-12,M0:316.967,Md:.0334442282};function Re(i,e){let t=i,n=t+y(e*Math.sin(g(t))*(1+e*Math.cos(g(t))));for(let a=0;a<8;a++){let o=g(n),r=(n-y(e*Math.sin(o))-t)/(1-e*Math.cos(o));if(n-=r,Math.abs(r)<1e-9)break}return n}function z(i,e){let t=g(u(i.N0+i.Nd*e)),n=g(i.i0+i.id*e),a=g(u(i.w0+i.wd*e)),o=i.a,r=i.e0+i.ed*e,s=u(i.M0+i.Md*e),c=g(Re(s,r)),l=o*(Math.cos(c)-r),m=o*(Math.sqrt(1-r*r)*Math.sin(c)),b=Math.atan2(m,l),h=Math.sqrt(l*l+m*m),d=b+a,p=h*(Math.cos(t)*Math.cos(d)-Math.sin(t)*Math.sin(d)*Math.cos(n)),D=h*(Math.sin(t)*Math.cos(d)+Math.cos(t)*Math.sin(d)*Math.cos(n)),_=h*(Math.sin(d)*Math.sin(n));return{xh:p,yh:D,zh:_,distanceAu:h,trueLonDeg:u(y(d))}}function L(i,e,t){let n=g(i),a=g(e),o=g(t),r=Math.sin(a)*Math.cos(o)+Math.cos(a)*Math.sin(o)*Math.sin(n),s=y(Math.asin(C(r,-1,1))),c=Math.sin(n)*Math.cos(o)-Math.tan(a)*Math.sin(o),l=Math.cos(n);return{raHours:u(y(Math.atan2(c,l)))/15,decDeg:s}}function ie(i){let e=R(i),t=z(K,e);return{...L(t.trueLonDeg,0,E(i)),eclipticLonDeg:t.trueLonDeg,distanceAu:t.distanceAu}}function oe(i){let e=R(i),t=z(K,e);return{xs:t.xh,ys:t.yh}}function G(i){let e=F(i),t=u(218.3164477+481267.88123421*e),n=u(297.8501921+445267.1114034*e),a=u(357.5291092+35999.0502909*e),o=u(134.9633964+477198.8675055*e),r=u(93.272095+483202.0175233*e),s=g(n),c=g(a),l=g(o),m=g(r),b=6.289*Math.sin(l)-1.274*Math.sin(l-2*s)+.658*Math.sin(2*s)-.186*Math.sin(c)-.059*Math.sin(2*l-2*s)-.057*Math.sin(l-2*s+c)+.053*Math.sin(l+2*s)+.046*Math.sin(2*s-c)+.041*Math.sin(l-c)-.035*Math.sin(s)-.031*Math.sin(l+c)-.015*Math.sin(2*m-2*s)+.011*Math.sin(l-4*s),h=5.128*Math.sin(m)+.281*Math.sin(l+m)+.278*Math.sin(l-m)+.173*Math.sin(2*s-m)+.055*Math.sin(2*s-l+m)+.046*Math.sin(2*s-l-m)+.033*Math.sin(2*s+m)+.017*Math.sin(2*l+m);return{lonDeg:u(t+b),latDeg:h}}function re(i){let{lonDeg:e,latDeg:t}=G(i);return{...L(e,t,E(i)),eclipticLonDeg:e,eclipticLatDeg:t}}var Dt=Date.UTC(2e3,0,6,18,14,0),se=29.530588853,St=se*864e5;function ze(i){return i<22.5?"new":i<67.5?"waxing_crescent":i<112.5?"first_quarter":i<157.5?"waxing_gibbous":i<202.5?"full":i<247.5?"waning_gibbous":i<292.5?"last_quarter":i<337.5?"waning_crescent":"new"}function le(i,e){let t=u(i-e),n=(1-Math.cos(g(t)))/2;return{elongationDeg:t,illuminatedFraction:n,waxing:t<180,phaseKey:ze(t)}}function ce(i){return u(180-i)/360*se}var me={venus:ee,mars:te,jupiter:ne,saturn:ae};function Ne(i,e){let t=R(e),n=z(me[i],t),{xs:a,ys:o}=oe(e),r=n.xh+a,s=n.yh+o,c=n.zh,l=u(y(Math.atan2(s,r))),m=y(Math.atan2(c,Math.sqrt(r*r+s*s))),b=Math.sqrt(r*r+s*s+c*c);return{...L(l,m,E(e)),name:i,heliocentricDistanceAu:n.distanceAu,geocentricDistanceAu:b,eclipticLonDeg:l}}function de(i){return Object.keys(me).map(e=>Ne(e,i))}var M=[[-.4,[.61,.7,1]],[0,[.83,.87,1]],[.4,[1,1,.98]],[.7,[1,.94,.8]],[1.2,[1,.82,.6]],[2,[1,.64,.45]]];function ue(i){if(i<=M[0][0])return M[0][1];if(i>=M[M.length-1][0])return M[M.length-1][1];for(let e=0;e<M.length-1;e++){let[t,n]=M[e],[a,o]=M[e+1];if(i>=t&&i<=a){let r=(i-t)/(a-t);return[n[0]+(o[0]-n[0])*r,n[1]+(o[1]-n[1])*r,n[2]+(o[2]-n[2])*r]}}return M[M.length-1][1]}function q(i,e,t){let n=C((t-i)/(e-i),0,1);return n*n*(3-2*n)}function ge(i){return q(2,-18,i)}function he(i){let e=q(-10,-2,i),t=1-q(4,12,i);return C(Math.min(e,t),0,1)}function be(i){return i>2?"day":i>-12?"twilight":"night"}var Oe=["N","NE","E","SE","S","SW","W","NW"];function Be(i){let e=Math.round(u(i)/45)%8;return Oe[e]}function pe(i,e,t){let n=t.filter(o=>o.altDeg>0);n.sort((o,r)=>r.altDeg-o.altDeg);let a=n[0];return{moonPhaseKey:i,daysToFullMoon:ce(e),brightestVisiblePlanet:a?{name:a.name,compass:Be(a.azDeg)}:null}}var fe=[{id:"sirius",name:"Sirius",con:"CMa",raH:6.7525,decDeg:-16.7161,mag:-1.46,bv:0},{id:"canopus",name:"Canopus",con:"Car",raH:6.3992,decDeg:-52.6957,mag:-.74,bv:.15},{id:"rigil_kent",name:"Rigil Kentaurus",con:"Cen",raH:14.66,decDeg:-60.8339,mag:-.27,bv:.71},{id:"arcturus",name:"Arcturus",con:"Boo",raH:14.261,decDeg:19.1825,mag:-.05,bv:1.23},{id:"vega",name:"Vega",con:"Lyr",raH:18.6157,decDeg:38.7837,mag:.03,bv:0},{id:"capella",name:"Capella",con:"Aur",raH:5.2782,decDeg:45.998,mag:.08,bv:.8},{id:"rigel",name:"Rigel",con:"Ori",raH:5.2423,decDeg:-8.2016,mag:.13,bv:-.03},{id:"procyon",name:"Procyon",con:"CMi",raH:7.655,decDeg:5.225,mag:.34,bv:.42},{id:"achernar",name:"Achernar",con:"Eri",raH:1.6286,decDeg:-57.2367,mag:.46,bv:-.16},{id:"betelgeuse",name:"Betelgeuse",con:"Ori",raH:5.9195,decDeg:7.4071,mag:.5,bv:1.85},{id:"hadar",name:"Hadar",con:"Cen",raH:14.0637,decDeg:-60.373,mag:.61,bv:-.23},{id:"altair",name:"Altair",con:"Aql",raH:19.8464,decDeg:8.8683,mag:.76,bv:.22},{id:"acrux",name:"Acrux",con:"Cru",raH:12.4433,decDeg:-63.0991,mag:.77,bv:-.24},{id:"aldebaran",name:"Aldebaran",con:"Tau",raH:4.5987,decDeg:16.5093,mag:.85,bv:1.54},{id:"antares",name:"Antares",con:"Sco",raH:16.4901,decDeg:-26.432,mag:.96,bv:1.83},{id:"spica",name:"Spica",con:"Vir",raH:13.4199,decDeg:-11.1613,mag:.97,bv:-.24},{id:"pollux",name:"Pollux",con:"Gem",raH:7.7553,decDeg:28.0262,mag:1.14,bv:1},{id:"fomalhaut",name:"Fomalhaut",con:"PsA",raH:22.9608,decDeg:-29.6222,mag:1.16,bv:.09},{id:"deneb",name:"Deneb",con:"Cyg",raH:20.6905,decDeg:45.2803,mag:1.25,bv:.09},{id:"mimosa",name:"Mimosa",con:"Cru",raH:12.7953,decDeg:-59.6888,mag:1.25,bv:-.23},{id:"regulus",name:"Regulus",con:"Leo",raH:10.1395,decDeg:11.9672,mag:1.36,bv:-.11},{id:"adhara",name:"Adhara",con:"CMa",raH:6.9771,decDeg:-28.9721,mag:1.5,bv:-.21},{id:"castor",name:"Castor",con:"Gem",raH:7.5766,decDeg:31.8883,mag:1.58,bv:.03},{id:"gacrux",name:"Gacrux",con:"Cru",raH:12.5194,decDeg:-57.1133,mag:1.63,bv:1.6},{id:"shaula",name:"Shaula",con:"Sco",raH:17.5601,decDeg:-37.1038,mag:1.63,bv:-.22},{id:"bellatrix",name:"Bellatrix",con:"Ori",raH:5.4188,decDeg:6.3497,mag:1.64,bv:-.22},{id:"elnath",name:"Elnath",con:"Tau",raH:5.4382,decDeg:28.6075,mag:1.65,bv:-.13},{id:"miaplacidus",name:"Miaplacidus",con:"Car",raH:9.2199,decDeg:-69.7172,mag:1.67,bv:0},{id:"alnilam",name:"Alnilam",con:"Ori",raH:5.6036,decDeg:-1.2019,mag:1.69,bv:-.18},{id:"alnitak",name:"Alnitak",con:"Ori",raH:5.6793,decDeg:-1.9426,mag:1.77,bv:-.2},{id:"mintaka",name:"Mintaka",con:"Ori",raH:5.5334,decDeg:-.2991,mag:2.23,bv:-.19},{id:"meissa",name:"Meissa",con:"Ori",raH:5.5851,decDeg:9.9342,mag:3.54,bv:-.18},{id:"saiph",name:"Saiph",con:"Ori",raH:5.7959,decDeg:-9.6696,mag:2.07,bv:-.17},{id:"alioth",name:"Alioth",con:"UMa",raH:12.9005,decDeg:55.9598,mag:1.76,bv:0},{id:"dubhe",name:"Dubhe",con:"UMa",raH:11.0621,decDeg:61.7511,mag:1.79,bv:1.07},{id:"merak",name:"Merak",con:"UMa",raH:11.0307,decDeg:56.3824,mag:2.34,bv:.03},{id:"phecda",name:"Phecda",con:"UMa",raH:11.8971,decDeg:53.6948,mag:2.44,bv:0},{id:"megrez",name:"Megrez",con:"UMa",raH:12.257,decDeg:57.0326,mag:3.32,bv:.08},{id:"mizar",name:"Mizar",con:"UMa",raH:13.3987,decDeg:54.9254,mag:2.23,bv:.02},{id:"alkaid",name:"Alkaid",con:"UMa",raH:13.7923,decDeg:49.3133,mag:1.85,bv:-.19},{id:"mirfak",name:"Mirfak",con:"Per",raH:3.4054,decDeg:49.8612,mag:1.79,bv:.48},{id:"algol",name:"Algol",con:"Per",raH:3.1361,decDeg:40.9556,mag:2.12,bv:-.05},{id:"atik",name:"Atik",con:"Per",raH:3.9021,decDeg:31.8836,mag:2.85,bv:-.19},{id:"wezen",name:"Wezen",con:"CMa",raH:7.1398,decDeg:-26.3932,mag:1.83,bv:.68},{id:"mirzam",name:"Mirzam",con:"CMa",raH:6.3783,decDeg:-17.9559,mag:1.98,bv:-.13},{id:"aludra",name:"Aludra",con:"CMa",raH:7.4013,decDeg:-29.3031,mag:2.45,bv:-.09},{id:"furud",name:"Furud",con:"CMa",raH:6.3399,decDeg:-30.0631,mag:3.02,bv:-.19},{id:"sargas",name:"Sargas",con:"Sco",raH:17.6222,decDeg:-42.9978,mag:1.87,bv:.4},{id:"dschubba",name:"Dschubba",con:"Sco",raH:16.0056,decDeg:-22.6217,mag:2.29,bv:-.12},{id:"larawag",name:"Larawag",con:"Sco",raH:16.8302,decDeg:-34.2932,mag:2.29,bv:1.15},{id:"graffias",name:"Graffias",con:"Sco",raH:16.0906,decDeg:-19.8054,mag:2.56,bv:-.07},{id:"girtab",name:"Girtab",con:"Sco",raH:17.7075,decDeg:-39.03,mag:2.39,bv:-.06},{id:"lesath",name:"Lesath",con:"Sco",raH:17.5128,decDeg:-37.2958,mag:2.7,bv:-.19},{id:"kaus_australis",name:"Kaus Australis",con:"Sgr",raH:18.4028,decDeg:-34.3846,mag:1.85,bv:.03},{id:"nunki",name:"Nunki",con:"Sgr",raH:18.9211,decDeg:-26.2967,mag:2.05,bv:-.18},{id:"kaus_media",name:"Kaus Media",con:"Sgr",raH:18.3502,decDeg:-29.8281,mag:2.7,bv:1.39},{id:"ascella",name:"Ascella",con:"Sgr",raH:19.0433,decDeg:-29.8801,mag:2.6,bv:.02},{id:"kaus_borealis",name:"Kaus Borealis",con:"Sgr",raH:18.4694,decDeg:-25.4217,mag:2.82,bv:1.04},{id:"alnasl",name:"Alnasl",con:"Sgr",raH:18.0973,decDeg:-30.4241,mag:2.98,bv:1.04},{id:"avior",name:"Avior",con:"Car",raH:8.3752,decDeg:-59.5095,mag:1.86,bv:1.28},{id:"menkalinan",name:"Menkalinan",con:"Aur",raH:5.9922,decDeg:44.9474,mag:1.9,bv:.03},{id:"atria",name:"Atria",con:"TrA",raH:16.8111,decDeg:-69.0277,mag:1.91,bv:1.44},{id:"alhena",name:"Alhena",con:"Gem",raH:6.6284,decDeg:16.3993,mag:1.93,bv:0},{id:"wasat",name:"Wasat",con:"Gem",raH:7.3378,decDeg:21.9825,mag:3.53,bv:.33},{id:"mebsuta",name:"Mebsuta",con:"Gem",raH:6.7333,decDeg:25.1311,mag:3.06,bv:1.44},{id:"tejat",name:"Tejat",con:"Gem",raH:6.383,decDeg:22.5137,mag:2.88,bv:1.64},{id:"peacock",name:"Peacock",con:"Pav",raH:20.4275,decDeg:-56.735,mag:1.94,bv:-.12},{id:"alsephina",name:"Alsephina",con:"Vel",raH:8.7455,decDeg:-54.7086,mag:1.96,bv:-.03},{id:"regor",name:"Regor",con:"Vel",raH:8.1594,decDeg:-47.3365,mag:1.75,bv:-.19},{id:"alphard",name:"Alphard",con:"Hya",raH:9.46,decDeg:-8.6586,mag:1.98,bv:1.44},{id:"hamal",name:"Hamal",con:"Ari",raH:2.1196,decDeg:23.4624,mag:2,bv:1.15},{id:"polaris",name:"Polaris",con:"UMi",raH:2.5303,decDeg:89.2641,mag:1.98,bv:.6},{id:"kochab",name:"Kochab",con:"UMi",raH:14.8451,decDeg:74.1555,mag:2.07,bv:1.47},{id:"pherkad",name:"Pherkad",con:"UMi",raH:15.3455,decDeg:71.834,mag:3.05,bv:.05},{id:"alnair",name:"Alnair",con:"Gru",raH:22.1372,decDeg:-46.9611,mag:1.74,bv:-.07},{id:"almach",name:"Almach",con:"And",raH:2.065,decDeg:42.3297,mag:2.1,bv:1.37},{id:"alpheratz",name:"Alpheratz",con:"And",raH:.1398,decDeg:29.0904,mag:2.06,bv:-.03},{id:"mirach",name:"Mirach",con:"And",raH:1.1622,decDeg:35.6206,mag:2.05,bv:1.58},{id:"denebola",name:"Denebola",con:"Leo",raH:11.8177,decDeg:14.572,mag:2.14,bv:.09},{id:"algieba",name:"Algieba",con:"Leo",raH:10.3327,decDeg:19.8415,mag:2.28,bv:1.14},{id:"zosma",name:"Zosma",con:"Leo",raH:11.2351,decDeg:20.5237,mag:2.56,bv:.13},{id:"chertan",name:"Chertan",con:"Leo",raH:11.2373,decDeg:15.4296,mag:3.34,bv:.03},{id:"muhlifain",name:"Muhlifain",con:"Cen",raH:12.6923,decDeg:-48.9598,mag:2.2,bv:0},{id:"menkent",name:"Menkent",con:"Cen",raH:14.1113,decDeg:-36.3699,mag:2.06,bv:.98},{id:"alphecca",name:"Alphecca",con:"CrB",raH:15.5782,decDeg:26.7147,mag:2.23,bv:.03},{id:"sadr",name:"Sadr",con:"Cyg",raH:20.3705,decDeg:40.2567,mag:2.23,bv:.67},{id:"aljanah",name:"Aljanah",con:"Cyg",raH:20.7702,decDeg:33.9702,mag:2.46,bv:1.03},{id:"fawaris",name:"Fawaris",con:"Cyg",raH:19.7497,decDeg:45.131,mag:2.87,bv:.05},{id:"albireo",name:"Albireo",con:"Cyg",raH:19.5121,decDeg:27.9597,mag:3.18,bv:1.14},{id:"eltanin",name:"Eltanin",con:"Dra",raH:17.9434,decDeg:51.4889,mag:2.23,bv:1.53},{id:"caph",name:"Caph",con:"Cas",raH:.1529,decDeg:59.1498,mag:2.27,bv:.38},{id:"schedar",name:"Schedar",con:"Cas",raH:.6751,decDeg:56.5373,mag:2.24,bv:1.17},{id:"tsih",name:"Tsih",con:"Cas",raH:.9451,decDeg:60.7167,mag:2.47,bv:-.15},{id:"ruchbah",name:"Ruchbah",con:"Cas",raH:1.43,decDeg:60.2353,mag:2.68,bv:.15},{id:"segin",name:"Segin",con:"Cas",raH:1.9061,decDeg:63.6701,mag:3.35,bv:-.11},{id:"ankaa",name:"Ankaa",con:"Phe",raH:.4383,decDeg:-42.3061,mag:2.4,bv:1.08},{id:"enif",name:"Enif",con:"Peg",raH:21.7364,decDeg:9.875,mag:2.38,bv:1.52},{id:"scheat",name:"Scheat",con:"Peg",raH:23.0629,decDeg:28.0828,mag:2.42,bv:1.67},{id:"markab",name:"Markab",con:"Peg",raH:23.0794,decDeg:15.2053,mag:2.49,bv:-.04},{id:"sabik",name:"Sabik",con:"Oph",raH:17.1728,decDeg:-15.7249,mag:2.43,bv:-.02},{id:"rasalhague",name:"Rasalhague",con:"Oph",raH:17.5824,decDeg:12.5601,mag:2.08,bv:.15},{id:"alderamin",name:"Alderamin",con:"Cep",raH:21.3096,decDeg:62.5856,mag:2.45,bv:.22},{id:"errai",name:"Errai",con:"Cep",raH:23.6564,decDeg:77.632,mag:3.21,bv:1.03},{id:"zubenelgenubi",name:"Zubenelgenubi",con:"Lib",raH:14.8479,decDeg:-16.0418,mag:2.75,bv:.15},{id:"zubeneschamali",name:"Zubeneschamali",con:"Lib",raH:15.2833,decDeg:-9.383,mag:2.61,bv:-.03},{id:"unukalhai",name:"Unukalhai",con:"Ser",raH:15.7382,decDeg:6.4256,mag:2.63,bv:1.17},{id:"menkar",name:"Menkar",con:"Cet",raH:3.0378,decDeg:4.09,mag:2.54,bv:1.64},{id:"diphda",name:"Diphda",con:"Cet",raH:.7265,decDeg:-17.9866,mag:2.04,bv:1.02},{id:"izar",name:"Izar",con:"Boo",raH:14.75,decDeg:27.0742,mag:2.35,bv:.96},{id:"seginus",name:"Seginus",con:"Boo",raH:14.535,decDeg:38.3082,mag:3.03,bv:.05},{id:"nekkar",name:"Nekkar",con:"Boo",raH:15.0322,decDeg:40.3906,mag:3.5,bv:.95},{id:"muphrid",name:"Muphrid",con:"Boo",raH:13.9144,decDeg:18.3976,mag:2.68,bv:.58},{id:"sheliak",name:"Sheliak",con:"Lyr",raH:18.8347,decDeg:33.3627,mag:3.45,bv:0},{id:"sulafat",name:"Sulafat",con:"Lyr",raH:18.9825,decDeg:32.6896,mag:3.24,bv:-.05},{id:"tarazed",name:"Tarazed",con:"Aql",raH:19.7709,decDeg:10.6133,mag:2.72,bv:1.53},{id:"alshain",name:"Alshain",con:"Aql",raH:19.9226,decDeg:6.4067,mag:3.71,bv:.86},{id:"imai",name:"Imai",con:"Cru",raH:12.2528,decDeg:-58.7489,mag:2.79,bv:-.13},{id:"alcyone",name:"Alcyone",con:"Tau",raH:3.7912,decDeg:24.1051,mag:2.87,bv:-.09}];var I=[{id:"UMa",labelKey:"con_ursa_major",segments:[["alkaid","mizar"],["mizar","alioth"],["alioth","megrez"],["megrez","phecda"],["phecda","merak"],["merak","dubhe"],["dubhe","megrez"]]},{id:"UMi",labelKey:"con_ursa_minor",segments:[["polaris","pherkad"],["pherkad","kochab"]]},{id:"Ori",labelKey:"con_orion",segments:[["betelgeuse","bellatrix"],["bellatrix","mintaka"],["betelgeuse","alnitak"],["mintaka","alnilam"],["alnilam","alnitak"],["mintaka","rigel"],["alnitak","saiph"],["bellatrix","meissa"],["betelgeuse","meissa"]]},{id:"Cas",labelKey:"con_cassiopeia",segments:[["segin","ruchbah"],["ruchbah","tsih"],["tsih","schedar"],["schedar","caph"]]},{id:"Cyg",labelKey:"con_cygnus",segments:[["deneb","sadr"],["sadr","aljanah"],["sadr","fawaris"],["sadr","albireo"]]},{id:"Leo",labelKey:"con_leo",segments:[["denebola","chertan"],["chertan","zosma"],["chertan","algieba"],["algieba","regulus"]]},{id:"Sco",labelKey:"con_scorpius",segments:[["graffias","dschubba"],["dschubba","antares"],["antares","larawag"],["larawag","sargas"],["sargas","girtab"],["girtab","shaula"],["shaula","lesath"]]},{id:"Sgr",labelKey:"con_sagittarius",segments:[["kaus_borealis","kaus_media"],["kaus_media","kaus_australis"],["kaus_australis","alnasl"],["kaus_borealis","nunki"],["nunki","ascella"]]},{id:"Tau",labelKey:"con_taurus",segments:[["aldebaran","elnath"],["aldebaran","alcyone"]]},{id:"Gem",labelKey:"con_gemini",segments:[["castor","pollux"],["castor","wasat"],["wasat","alhena"],["pollux","mebsuta"],["mebsuta","tejat"]]},{id:"CMa",labelKey:"con_canis_major",segments:[["sirius","mirzam"],["sirius","adhara"],["adhara","wezen"],["wezen","aludra"],["sirius","furud"]]},{id:"Cru",labelKey:"con_crux",segments:[["acrux","gacrux"],["mimosa","imai"]]},{id:"Cen",labelKey:"con_centaurus",segments:[["rigil_kent","hadar"],["hadar","menkent"],["hadar","muhlifain"]]},{id:"Boo",labelKey:"con_bootes",segments:[["arcturus","muphrid"],["arcturus","seginus"],["seginus","nekkar"],["arcturus","izar"]]},{id:"Lyr",labelKey:"con_lyra",segments:[["vega","sheliak"],["sheliak","sulafat"]]},{id:"Aql",labelKey:"con_aquila",segments:[["tarazed","altair"],["altair","alshain"]]},{id:"And",labelKey:"con_andromeda",segments:[["alpheratz","mirach"],["mirach","almach"]]},{id:"Per",labelKey:"con_perseus",segments:[["mirfak","algol"],["mirfak","atik"]]}];function ve(i,e){var _,T,X,$;let t=J(i,e.lonDeg),n=(f,v)=>{let{altDeg:P,azDeg:S}=Z(f,v,t,e.latDeg);return{...Q(P,S),altDeg:P,azDeg:S}},a=new Map,o=fe.map(f=>{let v=n(f.raH,f.decDeg),P={x:v.x,y:v.y,visible:v.visible,mag:f.mag,color:ue(f.bv),altDeg:v.altDeg,azDeg:v.azDeg};return a.set(f.id,P),P}),r=I.flatMap(f=>f.segments.map(([v,P])=>{let S=a.get(v),A=a.get(P);return!S||!A?{ax:0,ay:0,bx:0,by:0,visible:!1}:{ax:S.x,ay:S.y,bx:A.x,by:A.y,visible:S.visible&&A.visible}})),s=I.map(f=>{let v=new Set;for(let[x,k]of f.segments)v.add(x),v.add(k);let P=[...v].map(x=>a.get(x)).filter(x=>!!x),S=P.filter(x=>x.visible),A=S.length>0?S:P,Ce=A.reduce((x,k)=>x+k.x,0)/(A.length||1),He=A.reduce((x,k)=>x+k.y,0)/(A.length||1);return{labelKey:f.labelKey,x:Ce,y:He,visible:S.length>0}}),c=ie(i),l=n(c.raHours,c.decDeg),m=re(i),b=G(i),h=n(m.raHours,m.decDeg),d=le(b.lonDeg,c.eclipticLonDeg),p=de(i).map(f=>{let v=n(f.raHours,f.decDeg);return{x:v.x,y:v.y,visible:v.visible,altDeg:v.altDeg,azDeg:v.azDeg,name:f.name}}),D=pe(d.phaseKey,d.elongationDeg,p.map(f=>({name:f.name,altDeg:f.altDeg,azDeg:f.azDeg})));return{stars:o,lines:r,constellationLabels:s,moon:{x:h.x,y:h.y,visible:h.visible,altDeg:h.altDeg,azDeg:h.azDeg,phaseFraction:d.illuminatedFraction,waxing:d.waxing},planets:p,sun:{x:l.x,y:l.y,visible:l.visible,altDeg:l.altDeg,azDeg:l.azDeg},starVisibility:ge(l.altDeg),twilightWarmth:he(l.altDeg),mood:be(l.altDeg),banner:{moonPhaseKey:D.moonPhaseKey,daysToFullMoon:D.daysToFullMoon,planetName:(T=(_=D.brightestVisiblePlanet)==null?void 0:_.name)!=null?T:null,planetCompass:($=(X=D.brightestVisiblePlanet)==null?void 0:X.compass)!=null?$:null}}}var U=[{name:"Paris",lat:48.8566,lon:2.3522},{name:"London",lat:51.5072,lon:-.1276},{name:"Berlin",lat:52.52,lon:13.405},{name:"Madrid",lat:40.4168,lon:-3.7038},{name:"Rome",lat:41.9028,lon:12.4964},{name:"Lisbon",lat:38.7223,lon:-9.1393},{name:"Amsterdam",lat:52.3676,lon:4.9041},{name:"Brussels",lat:50.8503,lon:4.3517},{name:"Vienna",lat:48.2082,lon:16.3738},{name:"Zurich",lat:47.3769,lon:8.5417},{name:"Dublin",lat:53.3498,lon:-6.2603},{name:"Stockholm",lat:59.3293,lon:18.0686},{name:"Oslo",lat:59.9139,lon:10.7522},{name:"Copenhagen",lat:55.6761,lon:12.5683},{name:"Helsinki",lat:60.1699,lon:24.9384},{name:"Warsaw",lat:52.2297,lon:21.0122},{name:"Prague",lat:50.0755,lon:14.4378},{name:"Athens",lat:37.9838,lon:23.7275},{name:"Moscow",lat:55.7558,lon:37.6173},{name:"Istanbul",lat:41.0082,lon:28.9784},{name:"New York",lat:40.7128,lon:-74.006},{name:"Los Angeles",lat:34.0522,lon:-118.2437},{name:"Chicago",lat:41.8781,lon:-87.6298},{name:"Toronto",lat:43.6532,lon:-79.3832},{name:"Montreal",lat:45.5019,lon:-73.5674},{name:"Mexico City",lat:19.4326,lon:-99.1332},{name:"Sao Paulo",lat:-23.5505,lon:-46.6333},{name:"Rio de Janeiro",lat:-22.9068,lon:-43.1729},{name:"Buenos Aires",lat:-34.6037,lon:-58.3816},{name:"Santiago",lat:-33.4489,lon:-70.6693},{name:"Bogota",lat:4.711,lon:-74.0721},{name:"Cairo",lat:30.0444,lon:31.2357},{name:"Lagos",lat:6.5244,lon:3.3792},{name:"Nairobi",lat:-1.2921,lon:36.8219},{name:"Johannesburg",lat:-26.2041,lon:28.0473},{name:"Cape Town",lat:-33.9249,lon:18.4241},{name:"Dubai",lat:25.2048,lon:55.2708},{name:"Tel Aviv",lat:32.0853,lon:34.7818},{name:"Riyadh",lat:24.7136,lon:46.6753},{name:"Mumbai",lat:19.076,lon:72.8777},{name:"New Delhi",lat:28.6139,lon:77.209},{name:"Bangkok",lat:13.7563,lon:100.5018},{name:"Singapore",lat:1.3521,lon:103.8198},{name:"Jakarta",lat:-6.2088,lon:106.8456},{name:"Manila",lat:14.5995,lon:120.9842},{name:"Hong Kong",lat:22.3193,lon:114.1694},{name:"Shanghai",lat:31.2304,lon:121.4737},{name:"Beijing",lat:39.9042,lon:116.4074},{name:"Seoul",lat:37.5665,lon:126.978},{name:"Tokyo",lat:35.6762,lon:139.6503},{name:"Osaka",lat:34.6937,lon:135.5023},{name:"Sydney",lat:-33.8688,lon:151.2093},{name:"Melbourne",lat:-37.8136,lon:144.9631},{name:"Auckland",lat:-36.8485,lon:174.7633},{name:"Reykjavik",lat:64.1466,lon:-21.9426}],ye=U[0];function W(i){return i.trim().toLowerCase().normalize("NFD").replace(/[\u0300-\u036f]/g,"")}function De(i){if(!i||!i.trim())return ye;let e=W(i),t=U.find(a=>W(a.name)===e);if(t)return t;let n=U.find(a=>W(a.name).startsWith(e));return n!=null?n:ye}var Ve={con_ursa_major:{en:"Ursa Major",fr:"Grande Ourse",es:"Osa Mayor",de:"Grosser B\xE4r"},con_ursa_minor:{en:"Ursa Minor",fr:"Petite Ourse",es:"Osa Menor",de:"Kleiner B\xE4r"},con_orion:{en:"Orion",fr:"Orion",es:"Ori\xF3n",de:"Orion"},con_cassiopeia:{en:"Cassiopeia",fr:"Cassiop\xE9e",es:"Casiopea",de:"Kassiopeia"},con_cygnus:{en:"Cygnus",fr:"Cygne",es:"Cisne",de:"Schwan"},con_leo:{en:"Leo",fr:"Lion",es:"Leo",de:"L\xF6we"},con_scorpius:{en:"Scorpius",fr:"Scorpion",es:"Escorpio",de:"Skorpion"},con_sagittarius:{en:"Sagittarius",fr:"Sagittaire",es:"Sagitario",de:"Sch\xFCtze"},con_taurus:{en:"Taurus",fr:"Taureau",es:"Tauro",de:"Stier"},con_gemini:{en:"Gemini",fr:"G\xE9meaux",es:"G\xE9minis",de:"Zwillinge"},con_canis_major:{en:"Canis Major",fr:"Grand Chien",es:"Can Mayor",de:"Grosser Hund"},con_crux:{en:"Southern Cross",fr:"Croix du Sud",es:"Cruz del Sur",de:"Kreuz des S\xFCdens"},con_centaurus:{en:"Centaurus",fr:"Centaure",es:"Centauro",de:"Zentaur"},con_bootes:{en:"Bo\xF6tes",fr:"Bouvier",es:"Bootes",de:"Bootes"},con_lyra:{en:"Lyra",fr:"Lyre",es:"Lira",de:"Leier"},con_aquila:{en:"Aquila",fr:"Aigle",es:"\xC1guila",de:"Adler"},con_andromeda:{en:"Andromeda",fr:"Androm\xE8de",es:"Andr\xF3meda",de:"Andromeda"},con_perseus:{en:"Perseus",fr:"Pers\xE9e",es:"Perseo",de:"Perseus"},planet_venus:{en:"Venus",fr:"V\xE9nus",es:"Venus",de:"Venus"},planet_mars:{en:"Mars",fr:"Mars",es:"Marte",de:"Mars"},planet_jupiter:{en:"Jupiter",fr:"Jupiter",es:"J\xFApiter",de:"Jupiter"},planet_saturn:{en:"Saturn",fr:"Saturne",es:"Saturno",de:"Saturn"},phase_new:{en:"New moon",fr:"Nouvelle lune",es:"Luna nueva",de:"Neumond"},phase_waxing_crescent:{en:"Waxing crescent",fr:"Premier croissant",es:"Creciente",de:"Zunehmende Sichel"},phase_first_quarter:{en:"First quarter",fr:"Premier quartier",es:"Cuarto creciente",de:"Erstes Viertel"},phase_waxing_gibbous:{en:"Waxing gibbous",fr:"Gibbeuse croissante",es:"Gibosa creciente",de:"Zunehmender Mond"},phase_full:{en:"Full moon",fr:"Pleine lune",es:"Luna llena",de:"Vollmond"},phase_waning_gibbous:{en:"Waning gibbous",fr:"Gibbeuse d\xE9croissante",es:"Gibosa menguante",de:"Abnehmender Mond"},phase_last_quarter:{en:"Last quarter",fr:"Dernier quartier",es:"Cuarto menguante",de:"Letztes Viertel"},phase_waning_crescent:{en:"Waning crescent",fr:"Dernier croissant",es:"Menguante",de:"Abnehmende Sichel"},banner_tonight:{en:"Tonight",fr:"Ce soir",es:"Esta noche",de:"Heute Abend"},banner_planet_visible:{en:"visible",fr:"visible",es:"visible",de:"sichtbar"},banner_full_moon_in:{en:"Full moon in",fr:"Pleine lune dans",es:"Luna llena en",de:"Vollmond in"},banner_days_short:{en:"d",fr:"j",es:"d",de:"T"},banner_no_planet:{en:"No bright planet up right now",fr:"Aucune plan\xE8te visible pour l'instant",es:"Ning\xFAn planeta brillante visible ahora",de:"Gerade kein heller Planet sichtbar"},loading:{en:"Loading sky...",fr:"Chargement du ciel...",es:"Cargando el cielo...",de:"Himmel wird geladen..."},set_title:{en:"Night sky",fr:"Ciel nocturne",es:"Cielo nocturno",de:"Nachthimmel"},set_desc:{en:"Your location for the real-time planetarium widget.",fr:"Ta position pour le plan\xE9tarium temps r\xE9el.",es:"Tu ubicaci\xF3n para el planetario en tiempo real.",de:"Dein Standort f\xFCr das Echtzeit-Planetarium."},set_city:{en:"City",fr:"Ville",es:"Ciudad",de:"Stadt"},set_city_ph:{en:"e.g. Paris",fr:"ex. Paris",es:"ej. Par\xEDs",de:"z. B. Paris"},set_save:{en:"Save",fr:"Enregistrer",es:"Guardar",de:"Speichern"},set_saved:{en:"Sky settings saved",fr:"R\xE9glages du ciel enregistr\xE9s",es:"Ajustes del cielo guardados",de:"Himmel-Einstellungen gespeichert"},set_error:{en:"Couldn't save",fr:"Enregistrement impossible",es:"No se pudo guardar",de:"Speichern fehlgeschlagen"}};function w(i,e){var a;let t=Ve[i];if(!t)return i;let n=(e!=null?e:"en").slice(0,2).toLowerCase();return(a=t[n])!=null?a:t.en}var Se=`
.cos-root {
  position: relative;
  width: 100%;
  height: 100vh;
  border-radius: 16px;
  overflow: hidden;
  background: #05070f;
}
.cos-scene { position: absolute; inset: 0; cursor: default; }
.cos-banner {
  position: absolute;
  left: clamp(10px, 5cqi, 20px);
  bottom: clamp(8px, 4cqi, 16px);
  max-width: 85%;
  color: #f2f4ff;
  font-family: system-ui, -apple-system, BlinkMacSystemFont, sans-serif;
  font-size: clamp(11px, 3cqi, 14px);
  letter-spacing: 0.01em;
  text-shadow: 0 1px 4px rgba(0,0,0,0.7);
  pointer-events: none;
  user-select: none;
}
.cos-label {
  position: absolute;
  transform: translate(-50%, -100%);
  color: #ffffff;
  font-family: system-ui, -apple-system, BlinkMacSystemFont, sans-serif;
  font-size: clamp(10px, 2.6cqi, 13px);
  font-weight: 600;
  letter-spacing: 0.04em;
  text-shadow: 0 1px 4px rgba(0,0,0,0.85);
  pointer-events: none;
  white-space: nowrap;
  opacity: 0;
  transition: opacity 120ms ease;
}
.cos-label.visible { opacity: 1; }
`,Fe=Se.replace(/cqi/g,"vw");function xe(i){var b,h;let e=document.createElement("style"),t=((h=(b=window.CSS)==null?void 0:b.supports)==null?void 0:h.call(b,"width: 1cqi"))===!0;e.textContent=t?`.cos-root { container-type: inline-size; }
${Se}`:Fe,document.head.appendChild(e);let n=document.createElement("div");n.className="cos-root";let a=document.createElement("div");a.className="cos-scene",n.appendChild(a);let o=document.createElement("div");o.className="cos-banner",n.appendChild(o);let r=document.createElement("div");r.className="cos-label",n.appendChild(r),i.appendChild(n);let s=d=>{let p=a.getBoundingClientRect();l==null||l(d.clientX-p.left-p.width/2,d.clientY-p.top-p.height/2)},c=()=>m==null?void 0:m(),l=null,m=null;return a.addEventListener("pointermove",s),a.addEventListener("pointerdown",s),a.addEventListener("pointerleave",c),{canvasHost:a,setBanner(d){o.textContent=d},setHoverLabel(d,p,D){if(!d){r.classList.remove("visible");return}let _=a.getBoundingClientRect(),T=Math.min(_.width,_.height)/2;r.style.left=`${_.width/2+p*T}px`,r.style.top=`${_.height/2+D*T-10}px`,r.textContent=d,r.classList.add("visible")},onPointer(d,p){l=d,m=p},dispose(){a.removeEventListener("pointermove",s),a.removeEventListener("pointerdown",s),a.removeEventListener("pointerleave",c),n.remove(),e.remove()}}}var Ke=60*1e3,Ge="residence.city",qe=.14,O=class{constructor(e){this.deps=e;this.overlay=null;this.init=null;this.city="";this.coord={latDeg:48.8566,lonDeg:2.3522};this.labels=[];this.tickTimer=0;this.disposed=!1;this.onPageHide=()=>this.dispose();this.onResize=()=>this.requestHeight();this.onVisible=()=>{document.hidden||this.tick()}}start(){this.deps.bridge.onReady(e=>void this.boot(e))}dispose(){var e;this.disposed||(this.disposed=!0,clearInterval(this.tickTimer),window.removeEventListener("pagehide",this.onPageHide),window.removeEventListener("resize",this.onResize),document.removeEventListener("visibilitychange",this.onVisible),this.deps.renderer.dispose(),(e=this.overlay)==null||e.dispose(),this.overlay=null)}async boot(e){this.init=e,this.city=e.city,document.documentElement.style.background="transparent",document.body.style.margin="0",document.body.style.background="transparent",document.body.style.overflow="hidden",this.overlay=xe(document.body),this.deps.renderer.mount(this.overlay.canvasHost),this.overlay.onPointer((t,n)=>this.handlePointer(t,n),()=>{var t;return(t=this.overlay)==null?void 0:t.setHoverLabel(null,0,0)}),this.requestHeight(),window.addEventListener("resize",this.onResize),window.addEventListener("pagehide",this.onPageHide),document.addEventListener("visibilitychange",this.onVisible),await this.loadResidence(),!this.disposed&&(this.tick(),this.tickTimer=window.setInterval(()=>void this.refreshAndTick(),Ke))}async refreshAndTick(){await this.loadResidence(),!this.disposed&&this.tick()}async loadResidence(){let e=this.init;if(!e)return;try{let n=await this.deps.bridge.getPref(Ge);this.city=n&&n.trim()||e.city}catch{}let t=De(this.city);this.coord={latDeg:t.lat,lonDeg:t.lon}}requestHeight(){let e=window.innerWidth||document.documentElement.clientWidth||800,t=e<420?260:e<700?320:380;this.deps.bridge.resize(t)}tick(){var n;if(this.disposed||!this.overlay)return;let e=ve(Date.now(),this.coord);this.labels=e.constellationLabels,this.deps.renderer.setTarget(e);let t=(n=this.init)==null?void 0:n.locale;this.overlay.setBanner(this.buildBannerText(e.banner,t))}buildBannerText(e,t){let n=w("banner_tonight",t),a=e.planetName?`${w(`planet_${e.planetName}`,t)} ${w("banner_planet_visible",t)} ${e.planetCompass}`:w("banner_no_planet",t),o=Math.round(e.daysToFullMoon),r=o<=0?w("phase_full",t):`${w("banner_full_moon_in",t)} ${o}${w("banner_days_short",t)}`;return`${n} : ${a} \xB7 ${r}`}handlePointer(e,t){var m,b,h,d;let n=(m=this.overlay)==null?void 0:m.canvasHost;if(!n||this.labels.length===0)return;let a=n.getBoundingClientRect(),o=Math.min(a.width,a.height)/2;if(o<=0)return;let r=e/o,s=-t/o,c=null,l=qe;for(let p of this.labels){if(!p.visible)continue;let D=Math.hypot(p.x-r,p.y-s);D<l&&(l=D,c=p)}c?(h=this.overlay)==null||h.setHoverLabel(w(c.labelKey,(b=this.init)==null?void 0:b.locale),c.x,c.y):(d=this.overlay)==null||d.setHoverLabel(null,0,0)}};var B=class{onReady(e){let t=(n,a,o)=>{var r;return{city:(r=n==null?void 0:n.city)!=null?r:"Paris",locale:a!=null?a:"en",theme:o!=null?o:"dark"}};typeof window!="undefined"&&window.evey?window.evey.onInit(n=>e(t(n==null?void 0:n.config,n==null?void 0:n.locale,n==null?void 0:n.theme))):e(t(void 0,navigator==null?void 0:navigator.language,void 0))}resize(e){var t;(t=window.evey)==null||t.resize(Math.round(e))}async getPref(e){var n;let t=(n=window.evey)==null?void 0:n.prefs;if(!(t!=null&&t.get))return null;try{let a=await t.get(e);return a==null?null:typeof a=="string"?a:a.found===!1?null:typeof a.value=="string"?a.value:null}catch{return null}}async setPref(e,t){var a;let n=(a=window.evey)==null?void 0:a.prefs;if(n!=null&&n.set)try{await n.set(e,t)}catch{}}toast(e,t){var n,a;(a=(n=window.evey)==null?void 0:n.toast)==null||a.call(n,e,t)}};var Me=`
attribute vec2 aPos;
varying vec2 vUv;
void main() {
  vUv = aPos * 0.5 + 0.5;
  gl_Position = vec4(aPos, 0.0, 1.0);
}
`,Pe=`
#ifdef GL_FRAGMENT_PRECISION_HIGH
precision highp float;
#else
precision mediump float;
#endif

uniform vec2  uRes;
uniform float uTime;
uniform float uStarVis;   // 0 day .. 1 astronomical night
uniform float uTwilight;  // 0..1, peaks at sunset/sunrise
uniform vec2  uSunPos;    // dome-projected sun position, NDC-ish (-1..1), y up
uniform vec4  uShootStar; // x0,y0,x1,y1 in NDC
uniform float uShootProgress; // 0..1, <0 means inactive

varying vec2 vUv;

float hash21(vec2 p) {
  p = fract(p * vec2(123.34, 345.45));
  p += dot(p, p + 34.345);
  return fract(p.x * p.y);
}

float vnoise(vec2 p) {
  vec2 i = floor(p);
  vec2 f = fract(p);
  f = f * f * (3.0 - 2.0 * f);
  float a = hash21(i);
  float b = hash21(i + vec2(1.0, 0.0));
  float c = hash21(i + vec2(0.0, 1.0));
  float d = hash21(i + vec2(1.0, 1.0));
  return mix(mix(a, b, f.x), mix(c, d, f.x), f.y);
}

float fbm(vec2 p) {
  float v = 0.0;
  float a = 0.5;
  mat2 m = mat2(1.6, 1.2, -1.2, 1.6);
  for (int i = 0; i < 5; i++) {
    v += a * vnoise(p);
    p = m * p;
    a *= 0.5;
  }
  return v;
}

void main() {
  vec2 uv = vUv;
  float aspect = uRes.x / max(uRes.y, 1.0);
  vec2 p = (uv - 0.5) * vec2(aspect, 1.0) * 2.0;

  // 1. Base sky gradient: deep night blue -> twilight indigo/gold -> day blue.
  vec3 nightTop = vec3(0.02, 0.03, 0.07);
  vec3 nightBot = vec3(0.05, 0.06, 0.12);
  vec3 dayTop   = vec3(0.25, 0.55, 0.95);
  vec3 dayBot   = vec3(0.55, 0.75, 0.98);
  vec3 duskTop  = vec3(0.10, 0.08, 0.20);
  vec3 duskBot  = vec3(0.85, 0.45, 0.25);

  float dayness = 1.0 - uStarVis;
  vec3 top = mix(mix(nightTop, duskTop, uTwilight), dayTop, dayness);
  vec3 bot = mix(mix(nightBot, duskBot, uTwilight), dayBot, dayness);
  vec3 col = mix(bot, top, clamp(uv.y, 0.0, 1.0));

  // 2. Procedural Milky Way band: a soft diagonal fbm ridge, brighter where
  // the noise is denser, masked to a band and faded out in daylight.
  float bandAxis = (p.x + p.y * 1.3) * 0.6 + uTime * 0.002;
  float bandDist = abs(fract(bandAxis * 0.5 + 0.15) - 0.5) * 2.0;
  float band = smoothstep(0.85, 0.0, bandDist);
  float cloudDetail = fbm(p * 1.8 + vec2(0.0, uTime * 0.003));
  float milkyWay = band * (0.35 + 0.65 * cloudDetail) * uStarVis;
  col += vec3(0.65, 0.68, 0.80) * milkyWay * 0.35;

  // 3. Faint background sparkle (distinct from the explicit named-star
  // points drawn by the body program): a dense, cheap dust of sub-pixel
  // twinkle so the sky doesn't look empty between the named stars.
  vec2 gp = p * 140.0;
  vec2 gi = floor(gp);
  float starChance = hash21(gi);
  float twinkle = 0.5 + 0.5 * sin(uTime * (2.0 + hash21(gi + 5.1) * 3.0) + hash21(gi) * 6.283);
  float dust = step(0.9925, starChance) * twinkle;
  col += vec3(dust) * uStarVis * 0.5;

  // 4. Sun glow bleeding into the gradient near the horizon (golden hour).
  float sunGlow = exp(-length(p - uSunPos) * 1.4) * uTwilight * 0.5;
  col += vec3(1.0, 0.65, 0.35) * sunGlow;

  // 5. Rare shooting star: a short fading streak, CPU-scheduled.
  if (uShootProgress >= 0.0) {
    vec2 a = uShootStar.xy;
    vec2 b = uShootStar.xy + (uShootStar.zw - uShootStar.xy) * uShootProgress;
    vec2 ab = b - a;
    float len2 = max(dot(ab, ab), 1e-5);
    float t = clamp(dot(p - a, ab) / len2, 0.0, 1.0);
    vec2 proj = a + ab * t;
    float d = length(p - proj);
    float fade = (1.0 - uShootProgress) * smoothstep(0.0, 1.0, t);
    float streak = exp(-d * 220.0) * fade;
    col += vec3(1.0, 1.0, 0.95) * streak;
  }

  gl_FragColor = vec4(col, 1.0);
}
`,Ae=`
attribute vec2 aPos;
attribute vec3 aColor;
attribute float aSize;
attribute float aKind; // 0=line 1=star 2=planet 3=moon 4=sun
attribute float aSeed;

uniform float uTime;
uniform float uStarVis;

varying vec3 vColor;
varying float vKind;
varying float vAlpha;

void main() {
  gl_Position = vec4(aPos, 0.0, 1.0);
  vColor = aColor;
  vKind = aKind;

  float alpha = 1.0;
  if (aKind >= 0.5 && aKind < 1.5) {
    // Star: fade with real day/night visibility and twinkle per-star.
    float tw = 0.65 + 0.35 * sin(uTime * (1.3 + aSeed * 2.1) + aSeed * 6.283);
    alpha = uStarVis * tw;
    gl_PointSize = aSize * (0.85 + 0.3 * tw);
  } else if (aKind >= 3.5) {
    // Sun: only meaningful in daylight/twilight.
    alpha = 1.0;
    gl_PointSize = aSize;
  } else {
    gl_PointSize = aSize;
  }
  vAlpha = alpha;
}
`,_e=`
#ifdef GL_FRAGMENT_PRECISION_HIGH
precision highp float;
#else
precision mediump float;
#endif

uniform float uMoonPhase; // 0..1, 0.5 = full
uniform float uMoonWaxing; // 1.0 waxing, 0.0 waning

varying vec3 vColor;
varying float vKind;
varying float vAlpha;

void main() {
  if (vKind < 0.5) {
    // Constellation line: flat, faint stroke.
    gl_FragColor = vec4(vColor, 0.35 * vAlpha);
    return;
  }

  vec2 c = gl_PointCoord * 2.0 - 1.0;
  float r = length(c);

  if (vKind < 1.5) {
    // Star point sprite: soft core + gentle falloff.
    float core = smoothstep(1.0, 0.0, r);
    float glow = exp(-r * r * 3.0);
    float a = clamp(core * 1.2 + glow * 0.4, 0.0, 1.0) * vAlpha;
    gl_FragColor = vec4(vColor, a);
    return;
  }

  if (vKind < 2.5) {
    // Planet: brighter disc with a soft halo.
    float disc = smoothstep(0.55, 0.35, r);
    float halo = exp(-r * r * 2.0) * 0.5;
    float a = clamp(disc + halo, 0.0, 1.0) * vAlpha;
    gl_FragColor = vec4(vColor, a);
    return;
  }

  if (vKind < 3.5) {
    // Moon: procedural disc with a phase terminator (no photo texture).
    if (r > 1.0) discard;
    float shadowX = uMoonWaxing > 0.5 ? (1.0 - 2.0 * uMoonPhase) : (2.0 * uMoonPhase - 1.0);
    float side = uMoonWaxing > 0.5 ? -1.0 : 1.0;
    float lit = smoothstep(0.03, -0.03, side * (c.x - shadowX * 1.6));
    vec3 litColor = vec3(0.96, 0.96, 0.92);
    vec3 shadowColor = vec3(0.10, 0.11, 0.14);
    vec3 col = mix(shadowColor, litColor, lit);
    float edge = smoothstep(1.0, 0.9, r);
    gl_FragColor = vec4(col, edge * vAlpha);
    return;
  }

  // Sun: warm disc with rays.
  float disc = smoothstep(0.5, 0.3, r);
  float rays = exp(-r * r * 1.2) * 0.6;
  vec3 col = mix(vec3(1.0, 0.6, 0.25), vec3(1.0, 0.92, 0.6), disc);
  gl_FragColor = vec4(col, clamp(disc + rays, 0.0, 1.0) * vAlpha);
}
`;var H=null;function Ie(){var i;if(H)return H;try{let e=document.createElement("canvas"),t=e.getContext("webgl2")||e.getContext("webgl");if(!t)return H={scale:.5,fps:24,maxDpr:1};let n=t.getParameter(t.MAX_TEXTURE_SIZE),a=(i=navigator.hardwareConcurrency)!=null?i:2;return"drawBuffers"in t&&n>=8192&&a>=4?H={scale:1,fps:30,maxDpr:1.5}:n>=4096?H={scale:.75,fps:30,maxDpr:1.25}:H={scale:.5,fps:24,maxDpr:1}}catch{return H={scale:.5,fps:24,maxDpr:1}}}var we=0,We=1,Ue=2,je=3,Ye=4,j=8,Y=18,Xe=75,$e=.9,V=class{constructor(){this.host=null;this.canvas=null;this.gl=null;this.skyProgram=null;this.skyVbo=null;this.skyLoc={};this.bodyProgram=null;this.pointsVbo=null;this.linesVbo=null;this.bodyLoc={};this.bodyAttr={};this.pointCount=0;this.lineVertCount=0;this.quality=Ie();this.reducedMotion=typeof window!="undefined"&&window.matchMedia("(prefers-reduced-motion: reduce)").matches;this.starVisibility=0;this.twilightWarmth=0;this.sunX=0;this.sunY=0;this.moonPhase=.5;this.moonWaxing=1;this.shootTimerS=6+Math.random()*Y;this.shootActive=!1;this.shootProgress=-1;this.shootFrom=[0,0];this.shootTo=[0,0];this.raf=0;this.running=!1;this.inView=!0;this.lastTs=0;this.frameAcc=0;this.time=0;this.lost=!1;this.resizeObs=null;this.viewObs=null;this.onLost=e=>{e.preventDefault(),this.lost=!0};this.onRestored=()=>{this.lost=!1,this.initGL()};this.loop=e=>{if(!this.running)return;if(this.raf=requestAnimationFrame(this.loop),document.hidden||!this.inView||this.lost||!this.gl){this.lastTs=e;return}let t=Math.min((e-this.lastTs)/1e3,.25);this.lastTs=e,this.frameAcc+=t;let n=1/this.quality.fps;if(this.frameAcc<n)return;let a=Math.min(this.frameAcc,n*3);this.frameAcc=0;let o=this.reducedMotion?.25:1;this.time+=a*o,this.advanceShootingStar(a*o),this.draw()}}mount(e){this.host=e;let t=document.createElement("canvas");t.style.cssText="position:absolute;inset:0;width:100%;height:100%;display:block;",e.appendChild(t),this.canvas=t,t.addEventListener("webglcontextlost",this.onLost,!1),t.addEventListener("webglcontextrestored",this.onRestored,!1),this.initGL(),this.resizeObs=new ResizeObserver(()=>this.resize()),this.resizeObs.observe(e),this.viewObs=new IntersectionObserver(n=>{var a,o;this.inView=(o=(a=n[0])==null?void 0:a.isIntersecting)!=null?o:!0}),this.viewObs.observe(e),this.resize(),this.running=!0,this.raf=requestAnimationFrame(this.loop)}setTarget(e){this.starVisibility=e.starVisibility,this.twilightWarmth=e.twilightWarmth,this.sunX=e.sun.x,this.sunY=e.sun.y,this.moonPhase=e.moon.phaseFraction,this.moonWaxing=e.moon.waxing?1:0,this.uploadPoints(e),this.uploadLines(e),this.starVisibility<.4&&(this.shootActive=!1,this.shootProgress=-1)}dispose(){var n,a,o;this.running=!1,cancelAnimationFrame(this.raf),(n=this.resizeObs)==null||n.disconnect(),(a=this.viewObs)==null||a.disconnect(),this.resizeObs=null,this.viewObs=null;let{gl:e,canvas:t}=this;t&&(t.removeEventListener("webglcontextlost",this.onLost),t.removeEventListener("webglcontextrestored",this.onRestored)),e&&(this.skyProgram&&e.deleteProgram(this.skyProgram),this.bodyProgram&&e.deleteProgram(this.bodyProgram),this.skyVbo&&e.deleteBuffer(this.skyVbo),this.pointsVbo&&e.deleteBuffer(this.pointsVbo),this.linesVbo&&e.deleteBuffer(this.linesVbo),(o=e.getExtension("WEBGL_lose_context"))==null||o.loseContext()),t==null||t.remove(),this.gl=null,this.skyProgram=null,this.bodyProgram=null,this.skyVbo=null,this.pointsVbo=null,this.linesVbo=null,this.canvas=null,this.host=null}compile(e,t){let n=this.gl;if(!n)return null;let a=n.createShader(e);return a?(n.shaderSource(a,t),n.compileShader(a),n.getShaderParameter(a,n.COMPILE_STATUS)?a:(console.error("cosmos shader:",n.getShaderInfoLog(a)),n.deleteShader(a),null)):null}link(e,t){let n=this.gl;if(!n)return null;let a=this.compile(n.VERTEX_SHADER,e),o=this.compile(n.FRAGMENT_SHADER,t);if(!a||!o)return null;let r=n.createProgram();return r?(n.attachShader(r,a),n.attachShader(r,o),n.linkProgram(r),n.deleteShader(a),n.deleteShader(o),n.getProgramParameter(r,n.LINK_STATUS)?r:(console.error("cosmos link:",n.getProgramInfoLog(r)),n.deleteProgram(r),null)):null}initGL(){let e=this.canvas;if(!e)return;let t=e.getContext("webgl",{alpha:!0,premultipliedAlpha:!1,antialias:!1,depth:!1,stencil:!1,preserveDrawingBuffer:!1,powerPreference:"low-power"});if(t&&(this.gl=t,this.skyProgram=this.link(Me,Pe),this.bodyProgram=this.link(Ae,_e),!(!this.skyProgram||!this.bodyProgram))){this.skyVbo=t.createBuffer(),t.bindBuffer(t.ARRAY_BUFFER,this.skyVbo),t.bufferData(t.ARRAY_BUFFER,new Float32Array([-1,-1,3,-1,-1,3]),t.STATIC_DRAW);for(let n of["uRes","uTime","uStarVis","uTwilight","uSunPos","uShootStar","uShootProgress"])this.skyLoc[n]=t.getUniformLocation(this.skyProgram,n);this.pointsVbo=t.createBuffer(),this.linesVbo=t.createBuffer();for(let n of["uTime","uStarVis","uMoonPhase","uMoonWaxing"])this.bodyLoc[n]=t.getUniformLocation(this.bodyProgram,n);for(let n of["aPos","aColor","aSize","aKind","aSeed"])this.bodyAttr[n]=t.getAttribLocation(this.bodyProgram,n);t.enable(t.BLEND),t.blendFunc(t.SRC_ALPHA,t.ONE_MINUS_SRC_ALPHA),t.disable(t.DEPTH_TEST),t.clearColor(0,0,0,1)}}uploadPoints(e){let t=this.gl;if(!t||!this.pointsVbo)return;let n=[],a=(s,c,l,m,b,h,d)=>{l&&n.push(s,c,m[0],m[1],m[2],b,h,d)};for(let s=0;s<e.stars.length;s++){let c=e.stars[s],l=Math.max(1.5,6.5-c.mag*1.15)*(this.canvas?this.canvas.height/600:1);a(c.x,c.y,c.visible,c.color,l,We,s*.61803398875%1)}for(let s of e.planets){let c=Je[s.name],l=9*(this.canvas?this.canvas.height/600:1);a(s.x,s.y,s.visible,c,l,Ue,0)}let o=26*(this.canvas?this.canvas.height/600:1);a(e.moon.x,e.moon.y,e.moon.visible,[1,1,1],o,je,0);let r=34*(this.canvas?this.canvas.height/600:1);a(e.sun.x,e.sun.y,e.sun.visible,[1,.8,.4],r,Ye,0),this.pointCount=n.length/j,t.bindBuffer(t.ARRAY_BUFFER,this.pointsVbo),t.bufferData(t.ARRAY_BUFFER,new Float32Array(n),t.DYNAMIC_DRAW)}uploadLines(e){let t=this.gl;if(!t||!this.linesVbo)return;let n=[];for(let a of e.lines)a.visible&&(n.push(a.ax,a.ay,.55,.6,.75,0,we,0),n.push(a.bx,a.by,.55,.6,.75,0,we,0));this.lineVertCount=n.length/j,t.bindBuffer(t.ARRAY_BUFFER,this.linesVbo),t.bufferData(t.ARRAY_BUFFER,new Float32Array(n),t.DYNAMIC_DRAW)}resize(){let{host:e,canvas:t,gl:n}=this;if(!e||!t||!n)return;let a=Math.min(window.devicePixelRatio||1,this.quality.maxDpr)*this.quality.scale,o=Math.max(1,Math.round(e.clientWidth*a)),r=Math.max(1,Math.round(e.clientHeight*a));(t.width!==o||t.height!==r)&&(t.width=o,t.height=r,n.viewport(0,0,o,r))}advanceShootingStar(e){if(this.shootActive){this.shootProgress+=e/$e,this.shootProgress>=1&&(this.shootActive=!1,this.shootProgress=-1,this.shootTimerS=Y+Math.random()*(Xe-Y));return}if(!(this.starVisibility<.4)&&(this.shootTimerS-=e,this.shootTimerS<=0)){let t=Math.random()*Math.PI*2,n=.5+Math.random()*.4,a=(Math.random()-.5)*1.6,o=(Math.random()-.5)*1.6;this.shootFrom=[a,o],this.shootTo=[a+Math.cos(t)*n,o+Math.sin(t)*n],this.shootActive=!0,this.shootProgress=0}}draw(){let e=this.gl,t=this.canvas;if(!(!e||!t)){if(e.clear(e.COLOR_BUFFER_BIT),this.skyProgram){e.useProgram(this.skyProgram),e.bindBuffer(e.ARRAY_BUFFER,this.skyVbo);let n=e.getAttribLocation(this.skyProgram,"aPos");e.enableVertexAttribArray(n),e.vertexAttribPointer(n,2,e.FLOAT,!1,0,0);let a=this.skyLoc;e.uniform2f(a.uRes,t.width,t.height),e.uniform1f(a.uTime,this.time),e.uniform1f(a.uStarVis,this.starVisibility),e.uniform1f(a.uTwilight,this.twilightWarmth),e.uniform2f(a.uSunPos,this.sunX,this.sunY),e.uniform4f(a.uShootStar,this.shootFrom[0],this.shootFrom[1],this.shootTo[0],this.shootTo[1]),e.uniform1f(a.uShootProgress,this.shootActive?this.shootProgress:-1),e.drawArrays(e.TRIANGLES,0,3),e.disableVertexAttribArray(n)}if(this.bodyProgram){e.useProgram(this.bodyProgram),e.uniform1f(this.bodyLoc.uTime,this.time),e.uniform1f(this.bodyLoc.uStarVis,this.starVisibility),e.uniform1f(this.bodyLoc.uMoonPhase,this.moonPhase),e.uniform1f(this.bodyLoc.uMoonWaxing,this.moonWaxing);let n=a=>{e.bindBuffer(e.ARRAY_BUFFER,a);let o=j*4;e.enableVertexAttribArray(this.bodyAttr.aPos),e.vertexAttribPointer(this.bodyAttr.aPos,2,e.FLOAT,!1,o,0),e.enableVertexAttribArray(this.bodyAttr.aColor),e.vertexAttribPointer(this.bodyAttr.aColor,3,e.FLOAT,!1,o,8),e.enableVertexAttribArray(this.bodyAttr.aSize),e.vertexAttribPointer(this.bodyAttr.aSize,1,e.FLOAT,!1,o,20),e.enableVertexAttribArray(this.bodyAttr.aKind),e.vertexAttribPointer(this.bodyAttr.aKind,1,e.FLOAT,!1,o,24),e.enableVertexAttribArray(this.bodyAttr.aSeed),e.vertexAttribPointer(this.bodyAttr.aSeed,1,e.FLOAT,!1,o,28)};this.lineVertCount>0&&(n(this.linesVbo),e.drawArrays(e.LINES,0,this.lineVertCount)),this.pointCount>0&&(n(this.pointsVbo),e.drawArrays(e.POINTS,0,this.pointCount))}}}},Je={venus:[1,.96,.85],mars:[1,.55,.4],jupiter:[.95,.87,.72],saturn:[.95,.9,.7]};new O({renderer:new V,bridge:new B}).start();})();
