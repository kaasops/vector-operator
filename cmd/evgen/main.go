package main

import (
	"context"
	"flag"
	"fmt"
	"k8s.io/apimachinery/pkg/util/rand"
	ctrl "sigs.k8s.io/controller-runtime"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

func main() {
	numEvents := flag.Int("events", 300, "Number of events to create")
	namespace := flag.String("namespace", "default", "Namespace where the events will be created")
	workers := flag.Int("workers", 50, "Number of workers to create")
	flag.Parse()

	if *workers <= 0 {
		panic("Number of workers must be greater than 0")
	}

	config, err := ctrl.GetConfig()
	if err != nil {
		panic(err)
	}

	events := make(chan *corev1.Event)

	wg := sync.WaitGroup{}

	for i := 0; i < *workers; i++ {
		go func() {
			wg.Add(1)
			defer wg.Done()

			clientset, err := kubernetes.NewForConfig(config)
			if err != nil {
				panic(err)
			}

			for event := range events {
				_, err := clientset.CoreV1().Events(*namespace).Create(context.TODO(), event, metav1.CreateOptions{})
				if err != nil {
					fmt.Printf("Error creating event: %v\n", err)
				}
			}
		}()
	}

	start := time.Now()
	suffix := rand.String(3)

	for i := 0; i < *numEvents; i++ {
		eventName := fmt.Sprintf("test-event-%s-%d", suffix, i)

		event := &corev1.Event{
			ObjectMeta: metav1.ObjectMeta{
				Name:      eventName,
				Namespace: *namespace,
			},
			InvolvedObject: corev1.ObjectReference{
				Kind:      "Pod",
				Namespace: *namespace,
				Name:      "test-pod",
				UID:       "12345",
			},
			Reason:  "TestEvent",
			Message: fmt.Sprintf("This is test event number %d", i+1),
			Source: corev1.EventSource{
				Component: "event-generator",
			},
			FirstTimestamp: metav1.Time{Time: time.Now()},
			LastTimestamp:  metav1.Time{Time: time.Now()},
			Count:          1,
			Type:           "Normal",
		}
		events <- event
	}

	fmt.Printf("Event generation completed, elapsed time: %s", time.Since(start))
	close(events)
	wg.Wait()
}
