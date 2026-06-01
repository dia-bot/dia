defmodule Dia.Gateway.MixProject do
  use Mix.Project

  def project do
    [
      app: :dia_gateway,
      version: "0.1.0",
      elixir: "~> 1.18",
      elixirc_paths: elixirc_paths(Mix.env()),
      start_permanent: Mix.env() == :prod,
      deps: deps(),
      releases: releases()
    ]
  end

  # Run "mix help compile.app" to learn about applications.
  def application do
    [
      extra_applications: [:logger],
      mod: {Dia.Gateway.Application, []}
    ]
  end

  defp elixirc_paths(:test), do: ["lib", "test/support"]
  defp elixirc_paths(_), do: ["lib"]

  # Run "mix help deps" to learn about dependencies.
  defp deps do
    [
      # Discord gateway. We run it "thin": all caches are NoOp (see config.exs).
      # gun, certifi and jason are pulled in transitively.
      {:nostrum, "~> 0.10"},

      # NATS client. We use gnat's own JetStream API; the archived
      # :jetstream / mmmries package is intentionally NOT used.
      {:gnat, "~> 1.15"},
      {:jason, "~> 1.4"},

      # Clustering is compiled in but OFF by default (CLUSTER_ENABLED=true to turn on).
      {:libcluster, "~> 3.4"},
      {:telemetry, "~> 1.2"},
      {:telemetry_metrics, "~> 1.0"}
    ]
  end

  defp releases do
    [
      dia_gateway: [
        include_executables_for: [:unix],
        applications: [runtime_tools: :permanent]
      ]
    ]
  end
end
