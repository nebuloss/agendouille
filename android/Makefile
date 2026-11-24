# The agendouille APK, without Gradle (DECISIONS: no Gradle): aapt2 for the
# resources, javac against one android.jar, d8, zipalign and apksigner. Run
# on dev-build through scripts/android.sh, or in CI with the runner's SDK.
#
#   make apk     build/agendouille.apk, signed
#   make test    the JVM tests of the pure-Java parts
#   make cert    the signing certificate's SHA-256 (for assetlinks.json)
#
# Variables: SDK, JAVA_HOME, VERSION, VERSION_CODE, APP_HOST (the server's
# host, for App Links; empty: the agendouille:// scheme only), CLEARTEXT
# (true for a test server on plain http), KEYSTORE, KS_PASS, KEY_ALIAS.

SDK         ?= $(HOME)/android-build/sdk
JAVA_HOME   ?= $(firstword $(wildcard $(HOME)/android-build/jdk-*))
BUILD_TOOLS ?= $(lastword $(sort $(wildcard $(SDK)/build-tools/*)))
PLATFORM    ?= $(lastword $(sort $(wildcard $(SDK)/platforms/android-*)))/android.jar
LIB         ?= $(HOME)/android-build/lib
VERSION     ?= dev
VERSION_CODE ?= 1
APP_HOST    ?=
CLEARTEXT   ?= false
KEYSTORE    ?= $(HOME)/android-build/agendouille-debug.keystore
KS_PASS     ?= android
KEY_ALIAS   ?= agendouille

B      := build
JAVAC  := $(JAVA_HOME)/bin/javac
JAVA   := $(JAVA_HOME)/bin/java
export PATH := $(JAVA_HOME)/bin:$(PATH)

SRC    := $(shell find src -name '*.java')
RES    := $(shell find res -type f)
# The parts with no Android types, tested on the JVM.
PURE   := src/io/github/nebuloss/agendouille/EnrollLink.java \
          src/io/github/nebuloss/agendouille/ApiPath.java \
          src/io/github/nebuloss/agendouille/ServerAddress.java \
          src/io/github/nebuloss/agendouille/ICal.java \
          src/io/github/nebuloss/agendouille/EventMapper.java \
          src/io/github/nebuloss/agendouille/EventWriter.java \
          src/io/github/nebuloss/agendouille/Dav.java \
          src/io/github/nebuloss/agendouille/BridgeCodec.java \
          src/io/github/nebuloss/agendouille/Versions.java
TESTS  := $(shell find test -name '*Test.java')
TESTCP := $(LIB)/junit-4.13.2.jar:$(LIB)/hamcrest-core-1.3.jar:$(LIB)/json-20240303.jar:$(LIB)/kxml2-2.3.0.jar

.PHONY: apk test cert clean live-check
apk: $(B)/agendouille.apk

$(B)/AndroidManifest.xml: AndroidManifest.xml Makefile
	@mkdir -p $(B)
	@links=""; \
	if [ -n "$(APP_HOST)" ]; then \
	  links='      <intent-filter android:autoVerify="true">\n        <action android:name="android.intent.action.VIEW" />\n        <category android:name="android.intent.category.DEFAULT" />\n        <category android:name="android.intent.category.BROWSABLE" />\n        <data android:scheme="https" android:host="$(APP_HOST)" />\n        <data android:pathPrefix="/enroll/" />\n        <data android:pathPrefix="/enrol/" />\n      </intent-filter>'; \
	fi; \
	sed -e "s|@USES_CLEARTEXT@|$(CLEARTEXT)|" AndroidManifest.xml | awk -v links="$$links" '{ if ($$0 == "@APP_LINKS@") { gsub(/\\n/, "\n", links); print links } else print }' > $@

$(B)/res.zip: $(RES)
	@mkdir -p $(B)
	$(BUILD_TOOLS)/aapt2 compile --dir res -o $@

# The screens: src/android built by `npm run build` into assets/web.
ASSETS := $(shell find assets -type f)

$(B)/base.apk: $(B)/AndroidManifest.xml $(B)/res.zip $(ASSETS)
	@test -s assets/web/android.js || { echo "assets/web/android.js is missing: run npm run build first"; exit 1; }
	@rm -rf $(B)/gen && mkdir -p $(B)/gen
	$(BUILD_TOOLS)/aapt2 link -I $(PLATFORM) --manifest $(B)/AndroidManifest.xml -A assets \
	  --min-sdk-version 26 --target-sdk-version 34 \
	  --version-code $(VERSION_CODE) --version-name $(VERSION) \
	  --java $(B)/gen -o $@ $(B)/res.zip

$(B)/classes.stamp: $(SRC) $(B)/base.apk
	@rm -rf $(B)/classes && mkdir -p $(B)/classes
	$(JAVAC) -encoding UTF-8 -source 8 -target 8 -Xlint:-options -Xlint:deprecation \
	  -bootclasspath $(PLATFORM):$(BUILD_TOOLS)/core-lambda-stubs.jar -d $(B)/classes $(SRC) $$(find $(B)/gen -name '*.java')
	@touch $@

$(B)/classes.dex: $(B)/classes.stamp
	@rm -rf $(B)/dex && mkdir -p $(B)/dex
	$(BUILD_TOOLS)/d8 --release --min-api 26 --lib $(PLATFORM) --output $(B)/dex $$(find $(B)/classes -name '*.class')
	@cp $(B)/dex/classes.dex $@
	@# Phones without Google Play services are supported (DECISIONS): nothing
	@# of com.google.android.gms in the app's code, ever.
	@! LC_ALL=C grep -q 'com/google/android/gms' $@ || { echo "classes.dex refers to Google Play services"; rm -f $@; exit 1; }

$(B)/agendouille.apk: $(B)/base.apk $(B)/classes.dex $(KEYSTORE)
	@cp $(B)/base.apk $(B)/unaligned.apk
	@cd $(B) && zip -q -j unaligned.apk classes.dex
	$(BUILD_TOOLS)/zipalign -p -f 4 $(B)/unaligned.apk $(B)/aligned.apk
	$(BUILD_TOOLS)/apksigner sign --ks $(KEYSTORE) --ks-pass pass:$(KS_PASS) --ks-key-alias $(KEY_ALIAS) --out $@ $(B)/aligned.apk
	@rm -f $@.idsig
	@echo "built $@ ($(VERSION), code $(VERSION_CODE)$(if $(APP_HOST), App Links for $(APP_HOST)))"

# A debug key, made once on the build machine; a release key comes from CI.
$(KEYSTORE):
	@mkdir -p $(dir $@)
	$(JAVA_HOME)/bin/keytool -genkeypair -keystore $@ -storepass $(KS_PASS) -alias $(KEY_ALIAS) \
	  -keyalg RSA -keysize 3072 -validity 10000 -dname "CN=agendouille debug"

cert: $(KEYSTORE)
	@$(JAVA_HOME)/bin/keytool -list -v -keystore $(KEYSTORE) -storepass $(KS_PASS) -alias $(KEY_ALIAS) \
	  | awk '/SHA256:/ { print $$2; exit }'

test: $(LIB)/junit-4.13.2.jar $(LIB)/kxml2-2.3.0.jar
	@rm -rf $(B)/test && mkdir -p $(B)/test
	$(JAVAC) -encoding UTF-8 -cp $(TESTCP) -d $(B)/test $(PURE) $(TESTS)
	$(JAVA) -cp $(B)/test:$(TESTCP) org.junit.runner.JUnitCore $$(cd test && find . -name '*Test.java' | sed 's|^\./||; s|\.java$$||; s|/|.|g')

# JUnit, Hamcrest, org.json and kXML (Android's own XmlPullParser) for the
# JVM tests, from Maven Central, checked against their published SHA-1.
$(LIB)/kxml2-2.3.0.jar:
	@mkdir -p $(LIB)
	curl -sSfL -o $@ https://repo1.maven.org/maven2/net/sf/kxml/kxml2/2.3.0/kxml2-2.3.0.jar
	want=$$(curl -sSfL https://repo1.maven.org/maven2/net/sf/kxml/kxml2/2.3.0/kxml2-2.3.0.jar.sha1 | cut -c1-40); \
	got=$$(sha1sum $@ | cut -c1-40); [ "$$want" = "$$got" ] || { echo "checksum mismatch: kxml2"; rm -f $@; exit 1; }

$(LIB)/junit-4.13.2.jar:
	@mkdir -p $(LIB)
	curl -sSfL -o $(LIB)/junit-4.13.2.jar https://repo1.maven.org/maven2/junit/junit/4.13.2/junit-4.13.2.jar
	curl -sSfL -o $(LIB)/hamcrest-core-1.3.jar https://repo1.maven.org/maven2/org/hamcrest/hamcrest-core/1.3/hamcrest-core-1.3.jar
	curl -sSfL -o $(LIB)/json-20240303.jar https://repo1.maven.org/maven2/org/json/json/20240303/json-20240303.jar
	cd $(LIB) && for j in junit/junit/4.13.2/junit-4.13.2 org/hamcrest/hamcrest-core/1.3/hamcrest-core-1.3 org/json/json/20240303/json-20240303; do \
	  want=$$(curl -sSfL https://repo1.maven.org/maven2/$$j.jar.sha1 | cut -c1-40); \
	  got=$$(sha1sum $$(basename $$j).jar | cut -c1-40); \
	  [ "$$want" = "$$got" ] || { echo "checksum mismatch: $$j"; rm -f $$(basename $$j).jar; exit 1; }; \
	done

# The sync code against a running server (scripts/android-live.sh).
live-check: $(LIB)/junit-4.13.2.jar $(LIB)/kxml2-2.3.0.jar
	@rm -rf $(B)/live && mkdir -p $(B)/live
	$(JAVAC) -encoding UTF-8 -cp $(TESTCP) -d $(B)/live $(PURE) src/io/github/nebuloss/agendouille/RawHttp.java test/io/github/nebuloss/agendouille/LiveCheck.java
	$(JAVA) -cp $(B)/live:$(TESTCP) io.github.nebuloss.agendouille.LiveCheck $(SERVER) $(LIVE_USER) $(LIVE_PASS)

clean:
	rm -rf $(B)
