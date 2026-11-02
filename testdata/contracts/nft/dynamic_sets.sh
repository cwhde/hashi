set -u
nft delete table inet hashi 2>/dev/null
nft -f /root/shapes.nft || exit 1
echo "## add staging elements (as the kernel would, with set default timeout)"
nft add element inet hashi staging4 '{ 198.51.100.7, 198.51.100.8 }'
nft add element inet hashi staging6 '{ 2001:db8:5::1 }'
echo "## list staging4 json"; nft -j list set inet hashi staging4
echo "## list staging6 json"; nft -j list set inet hashi staging6
echo "## empty set"; nft delete element inet hashi staging6 '{ 2001:db8:5::1 }'; nft -j list set inet hashi staging6
cat > /root/push.json <<'J'
{"nftables":[
{"add":{"element":{"family":"inet","table":"hashi","name":"dyn4","elem":[{"elem":{"val":"198.51.100.7","timeout":900}},{"elem":{"val":"198.51.100.8","timeout":1800}}]}}},
{"add":{"element":{"family":"inet","table":"hashi","name":"dyn6","elem":[{"elem":{"val":{"prefix":{"addr":"2001:db8:5::","len":64}},"timeout":900}}]}}},
{"delete":{"element":{"family":"inet","table":"hashi","name":"staging4","elem":["198.51.100.7","198.51.100.8"]}}},
{"delete":{"element":{"family":"inet","table":"hashi","name":"dyn4","elem":["203.0.113.9"]}}}
]}
J
echo "## push batch"; nft -j -f /root/push.json; echo "rc=$?"
echo "## list dyn4 json"; nft -j list set inet hashi dyn4
echo "## list dyn6 json"; nft -j list set inet hashi dyn6
echo "## permanent (no timeout) on a timeout set"
echo '{"nftables":[{"add":{"element":{"family":"inet","table":"hashi","name":"dyn4","elem":["198.51.100.20"]}}}]}' | nft -j -f -; echo "rc=$?"
nft -j list set inet hashi dyn4
echo "## re-add existing element with new timeout"
echo '{"nftables":[{"add":{"element":{"family":"inet","table":"hashi","name":"dyn4","elem":[{"elem":{"val":"198.51.100.7","timeout":60}}]}}}]}' | nft -j -f -; echo "rc=$?"
nft -j list set inet hashi dyn4
echo "## delete missing element (whole batch must fail)"
echo '{"nftables":[{"add":{"element":{"family":"inet","table":"hashi","name":"dyn4","elem":["198.51.100.30"]}}},{"delete":{"element":{"family":"inet","table":"hashi","name":"dyn4","elem":["198.51.100.99"]}}}]}' | nft -j -f -; echo "rc=$?"
nft -j list set inet hashi dyn4
echo "## destroy (no error if missing)"
echo '{"nftables":[{"destroy":{"element":{"family":"inet","table":"hashi","name":"dyn4","elem":["198.51.100.99"]}}}]}' | nft -j -f -; echo "rc=$?"
echo "## missing set"
nft -j list set inet hashi nope; echo "rc=$?"
echo "## kernel adds to staging through a real meter? (skip, needs traffic)"
nft delete table inet hashi
