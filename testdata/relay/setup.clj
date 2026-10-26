;; Makes a relay data directory for relay-tui's tests against the real
;; relay, on artificial data: a role, and the cards a role would add.
;; Usage: bb setup.clj <relay checkout> <data dir>
(require '[babashka.classpath :as cp])
(let [[relay] *command-line-args*]
  (cp/add-classpath (str relay "/src:" relay "/resources")))
(require '[relay.inbound :as inbound] '[relay.log :as log])
(let [[_ data] *command-line-args*]
  (log/init! data)
  (spit (str data "/relay.edn")
        (pr-str {:timezone "UTC" :default-harness :a
                 :harnesses {:a {:kind :agentloop :api-key-command ["true"] :bin "/usr/bin/false"}}}))
  (let [sys (inbound/system data)]
    (inbound/submit! sys {:type :role/add :actor "operator" :args {:id "clerk" :title "Clerk" :instructions "x"}})
    ;; Outside text, with a link, a lookalike letter and an amount.
    (inbound/submit! sys {:type :card/add :actor "clerk" :labels ["contaminated" "private"]
                          :args {:kind "checklist" :title "Read https://evil.example/x?d=1"
                                 :items ["pаypal login" "wire 12.50 to acct"]}})
    ;; The operator's own, to archive.
    (inbound/submit! sys {:type :card/add :actor "clerk" :labels ["private"]
                          :args {:kind "checklist" :title "Pack for the trip" :items ["tent"]}})
    (println "ok")))
