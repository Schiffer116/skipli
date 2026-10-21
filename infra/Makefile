.PHONY: diagram

ENV ?= dev

# Renders architecture.png from a real plan of $(ENV), so it needs AWS access.
# TerraVision's layout is re-laid out left to right by diagram.gvpr, and
# labels.gvpr then puts each edge label beside the middle of its edge.
diagram: export AWS_PROFILE ?= iamadmin-general
diagram:
	$(MAKE) -C ../api build
	terraform init -input=false -reconfigure -backend-config=env/$(ENV).s3.tfbackend
	tmp=$$(mktemp -d) && \
	terraform plan -input=false -lock=false -var-file=env/$(ENV).tfvars -out=$$tmp/tfplan && \
	terraform show -json $$tmp/tfplan > $$tmp/plan.json && \
	terraform graph > $$tmp/graph.dot && \
	uvx terravision@0.56.0 draw --source ../infra --planfile $$tmp/plan.json --graphfile $$tmp/graph.dot --format dot --outfile $$tmp/architecture && \
	gvpr -c -f diagram.gvpr -o $$tmp/compact.dot $$tmp/architecture-aws.dot.dot && \
	dot -Tdot $$tmp/compact.dot | gvpr -c -f labels.gvpr | neato -n2 -Tpng -o architecture.png && \
	rm -rf $$tmp
